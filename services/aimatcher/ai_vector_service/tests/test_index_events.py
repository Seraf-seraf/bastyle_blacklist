import base64
import json
import uuid

from ai_vector_service.index_events import (
    AIVectorEventApplier,
    IndexCheckpoint,
    IndexSynchronizer,
    OutboxEvent,
    _row_to_event,
)


def test_synchronizer_applies_events_and_updates_checkpoint():
    reader = MemoryEventReader(
        [
            OutboxEvent(
                transaction_id="1",
                offset=1,
                event_type="media.ban.created.v1",
                aggregate_uid=str(uuid.uuid4()),
                payload={},
            ),
            OutboxEvent(
                transaction_id="1",
                offset=2,
                event_type="unrelated.v1",
                aggregate_uid=str(uuid.uuid4()),
                payload={},
            ),
            OutboxEvent(
                transaction_id="2",
                offset=1,
                event_type="media.ban.created.v1",
                aggregate_uid=str(uuid.uuid4()),
                payload={},
            ),
        ]
    )
    checkpoints = MemoryCheckpointStore()
    applier = MemoryApplier()
    synchronizer = IndexSynchronizer(
        consumer_id="ai-replica-1",
        reader=reader,
        checkpoints=checkpoints,
        applier=applier,
        batch_size=2,
    )

    synchronizer.catch_up()

    assert applier.applied == ["1/1", "2/1"]
    checkpoint = checkpoints.checkpoint
    assert checkpoint.last_transaction_id == "2"
    assert checkpoint.last_offset == 1
    assert checkpoint.stale is False


def test_synchronizer_marks_stale_on_apply_error():
    event_uid = str(uuid.uuid4())
    reader = MemoryEventReader(
        [
            OutboxEvent(
                transaction_id="1",
                offset=1,
                event_type="media.ban.created.v1",
                aggregate_uid=event_uid,
                payload={},
            )
        ]
    )
    checkpoints = MemoryCheckpointStore()
    applier = MemoryApplier(fail=True)
    synchronizer = IndexSynchronizer(
        consumer_id="ai-replica-1",
        reader=reader,
        checkpoints=checkpoints,
        applier=applier,
        batch_size=10,
    )

    try:
        synchronizer.catch_up()
    except RuntimeError:
        pass
    else:
        raise AssertionError("ожидалась ошибка применения события")

    assert checkpoints.checkpoint.last_transaction_id == "0"
    assert checkpoints.checkpoint.last_offset == 0
    assert checkpoints.checkpoint.stale is True
    assert event_uid in checkpoints.checkpoint.stale_reason


def test_ai_vector_applier_loads_saved_ban_by_uid():
    ban_uid = str(uuid.uuid4())
    service = MemoryVectorIndexService()
    applier = AIVectorEventApplier(service)

    applier.apply(
        OutboxEvent(
            transaction_id="1",
            offset=1,
            event_type="media.ban.created.v1",
            aggregate_uid=ban_uid,
            payload={},
        )
    )

    assert service.applied_ban_uids == [ban_uid]


def test_rabbitmq_consumer_runs_bootstrap_catch_up_before_background_thread(monkeypatch):
    order = []
    synchronizer = MemorySynchronizer(order)

    def fake_run(self):
        order.append("run")

    monkeypatch.setattr("ai_vector_service.index_events.RabbitMQIndexConsumer.run", fake_run)
    consumer = _rabbitmq_consumer(synchronizer)

    thread = consumer.start_background()
    thread.join(timeout=1)

    assert order == ["catch_up", "run"]


def test_rabbitmq_consumer_does_not_start_thread_after_bootstrap_error(monkeypatch):
    order = []
    synchronizer = MemorySynchronizer(order, fail=True)

    def fake_run(self):
        raise AssertionError("consumer thread не должен запускаться после ошибки bootstrap")

    monkeypatch.setattr("ai_vector_service.index_events.RabbitMQIndexConsumer.run", fake_run)
    consumer = _rabbitmq_consumer(synchronizer)

    try:
        consumer.start_background()
    except RuntimeError as err:
        assert str(err) == "bootstrap failed"
    else:
        raise AssertionError("ожидалась ошибка bootstrap")

    assert order == ["catch_up"]


def test_row_to_event_unwraps_watermill_forwarder_envelope():
    event_uid = str(uuid.uuid4())
    aggregate_uid = str(uuid.uuid4())
    payload = {"ban_uid": aggregate_uid}
    envelope = {
        "destination_topic": "media.ban.created.v1",
        "uuid": event_uid,
        "payload": base64.b64encode(json.dumps(payload).encode("utf-8")).decode("ascii"),
        "metadata": {
            "event_uid": event_uid,
            "event_type": "media.ban.created.v1",
            "aggregate_type": "media_ban",
            "aggregate_uid": aggregate_uid,
        },
    }

    event = _row_to_event(
        {
            "transaction_id": "1",
            "offset": 2,
            "payload": json.dumps(envelope).encode("utf-8"),
            "metadata": None,
        }
    )

    assert event.event_type == "media.ban.created.v1"
    assert event.aggregate_uid == aggregate_uid
    assert event.payload == payload


class MemoryEventReader:
    def __init__(self, events):
        self.events = events

    def load_after(self, transaction_id, offset, limit):
        result = []
        for event in self.events:
            if _event_after(event, transaction_id, offset):
                result.append(event)
            if len(result) == limit:
                break
        return result


class MemoryCheckpointStore:
    def __init__(self):
        self.checkpoint = IndexCheckpoint(
            consumer_id="ai-replica-1",
            index_name="ai_vector",
            last_transaction_id="0",
            last_offset=0,
            stale=False,
            stale_reason="",
        )

    def get_or_create(self, consumer_id, index_name):
        self.checkpoint.consumer_id = consumer_id
        self.checkpoint.index_name = index_name
        return self.checkpoint

    def update(self, consumer_id, index_name, transaction_id, offset):
        self.checkpoint.consumer_id = consumer_id
        self.checkpoint.index_name = index_name
        self.checkpoint.last_transaction_id = transaction_id
        self.checkpoint.last_offset = offset
        self.checkpoint.stale = False
        self.checkpoint.stale_reason = ""

    def mark_stale(self, consumer_id, index_name, reason):
        self.checkpoint.consumer_id = consumer_id
        self.checkpoint.index_name = index_name
        self.checkpoint.stale = True
        self.checkpoint.stale_reason = reason


class MemoryApplier:
    index_name = "ai_vector"

    def __init__(self, fail=False):
        self.fail = fail
        self.applied = []

    def supports(self, event_type):
        return event_type == "media.ban.created.v1"

    def apply(self, event):
        if self.fail:
            raise RuntimeError(f"ошибка применения {event.aggregate_uid}")
        self.applied.append(f"{event.transaction_id}/{event.offset}")


class MemoryVectorIndexService:
    def __init__(self):
        self.applied_ban_uids = []

    def apply_existing_ban(self, *, ban_uid):
        self.applied_ban_uids.append(ban_uid)
        return "applied"


class MemorySynchronizer:
    def __init__(self, order, fail=False):
        self.order = order
        self.fail = fail

    def catch_up(self):
        self.order.append("catch_up")
        if self.fail:
            raise RuntimeError("bootstrap failed")


def _rabbitmq_consumer(synchronizer):
    from ai_vector_service.index_events import RabbitMQIndexConsumer

    return RabbitMQIndexConsumer(
        rabbitmq_url="amqp://guest:guest@localhost:5672/",
        exchange="bastyle.events",
        exchange_type="topic",
        queue_template="bastyle.replica.%s.events",
        routing_keys=["media.ban.#"],
        prefetch=1,
        synchronizer=synchronizer,
        consumer_id="ai-replica-1",
        reconnect_interval=0.1,
        catch_up_interval=0.1,
    )


def _event_after(event, transaction_id, offset):
    if int(event.transaction_id) > int(transaction_id):
        return True
    return event.transaction_id == transaction_id and event.offset > offset

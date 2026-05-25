from collections.abc import Callable
from time import monotonic

from prometheus_client import CONTENT_TYPE_LATEST, Counter, Gauge, Histogram, generate_latest
from starlette.requests import Request
from starlette.responses import Response


HTTP_DURATION_BUCKETS = (0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30)
DB_DURATION_BUCKETS = (0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5)


http_requests_total = Counter(
    "bastyle_ai_http_requests_total",
    "Количество HTTP-запросов AI matcher.",
    ["method", "route", "status"],
)
http_request_duration_seconds = Histogram(
    "bastyle_ai_http_request_duration_seconds",
    "Длительность HTTP-запросов AI matcher.",
    ["method", "route", "status"],
    buckets=HTTP_DURATION_BUCKETS,
)
db_errors_total = Counter(
    "bastyle_ai_db_errors_total",
    "Количество ошибок PostgreSQL в AI matcher.",
    ["operation"],
)
db_query_duration_seconds = Histogram(
    "bastyle_ai_db_query_duration_seconds",
    "Длительность PostgreSQL-операций AI matcher.",
    ["operation"],
    buckets=DB_DURATION_BUCKETS,
)
index_stale = Gauge(
    "bastyle_ai_index_stale",
    "Признак stale-состояния AI-vector индекса.",
    ["index_name", "consumer_id"],
)
index_rebuild_duration_seconds = Gauge(
    "bastyle_ai_index_rebuild_duration_seconds",
    "Длительность последней пересборки FAISS-индекса.",
    ["model_name", "model_revision"],
)
index_vectors_total = Gauge(
    "bastyle_ai_index_vectors_total",
    "Количество векторов в FAISS-индексе.",
    ["model_name", "model_revision"],
)


def observe_db_operation(operation: str, callback: Callable[[], object]) -> object:
    started_at = monotonic()
    try:
        return callback()
    except Exception:
        db_errors_total.labels(operation=operation).inc()
        raise
    finally:
        db_query_duration_seconds.labels(operation=operation).observe(monotonic() - started_at)


def observe_http_request(request: Request, status_code: int, duration_seconds: float) -> None:
    if request.url.path == "/metrics":
        return

    labels = {
        "method": request.method,
        "route": _route_template(request),
        "status": str(status_code),
    }
    http_requests_total.labels(**labels).inc()
    http_request_duration_seconds.labels(**labels).observe(duration_seconds)


def metrics_response() -> Response:
    return Response(generate_latest(), media_type=CONTENT_TYPE_LATEST)


def _route_template(request: Request) -> str:
    route = request.scope.get("route")
    path = getattr(route, "path", None)
    if isinstance(path, str) and path:
        return path

    return "unmatched"

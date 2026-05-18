REPO_ROOT := ../..
TEST_PATH := services/aimatcher/ai_vector_service/tests
PYTHONPATH := services/aimatcher

.PHONY: help test

help:
	@echo "Доступные команды ai matcher:"
	@echo "  make test - запустить все тесты AI matcher"

test:
	cd $(REPO_ROOT) && PYTHONPATH=$(PYTHONPATH) .venv/bin/python -m pytest $(TEST_PATH)

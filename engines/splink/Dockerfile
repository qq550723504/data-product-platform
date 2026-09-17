FROM python:3.12-slim

ENV PYTHONDONTWRITEBYTECODE=1 \
    PYTHONUNBUFFERED=1 \
    PIP_NO_CACHE_DIR=1

WORKDIR /app

COPY pyproject.toml /app/pyproject.toml
RUN pip install --upgrade pip && pip install .

COPY app /app/app
COPY models /app/models

# The reference model is a generated statistical artifact. Build it from the
# committed, labelled fixtures with the pinned Splink runtime instead of
# hand-editing or silently shipping an untrained model.
RUN python -m app.training --model-dir /app/models/park-company-v1

EXPOSE 8091

CMD ["uvicorn", "app.main:app", "--host", "0.0.0.0", "--port", "8091"]

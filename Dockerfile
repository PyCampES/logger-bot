FROM python:3.14-alpine

COPY --from=ghcr.io/astral-sh/uv:latest /uv /usr/local/bin/uv

WORKDIR /app
COPY . .

# RUN uv sync

ENTRYPOINT ["uv", "run", "python", "src/logger_bot/main.py"]
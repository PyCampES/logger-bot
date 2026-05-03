FROM python:3.14-slim

COPY --from=ghcr.io/astral-sh/uv:latest /uv /usr/local/bin/uv
RUN apt-get update && apt-get install -y ffmpeg && rm -rf /var/lib/apt/lists/*

WORKDIR /app
COPY . .

RUN uv sync --locked --no-editable


ENTRYPOINT ["uv", "run", "--no-sync", "python", "-u", "src/logger_bot/main.py"]

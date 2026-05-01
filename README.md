# Logger Bot

A Telegram bot that accepts voice messages, transcribes them using OpenAI Whisper, and parses workout data (exercise, reps, weight) for logging to a Google Sheet.

## Architecture 
![](./imgs/architecture.mmd)

## Requirements

- Python >= 3.12
- [`uv`](https://github.com/astral-sh/uv) package manager
- A Telegram bot token (from [@BotFather](https://t.me/BotFather))
- A Google service account with access to a Google Sheet named `"Logueala"`

## Setup

1. **Install dependencies**
   ```bash
   uv sync
   ```

2. **Configure the bot token**

   Create a `.env` file in the project root:
   ```
   TELEGRAM_API_TOKEN=your_telegram_bot_token_here
   ```

3. **Configure Google Sheets access**

   Place your service account key file at the project root as `credentials.json`, and share the `"Logueala"` spreadsheet with the service account email.

## Running the bot

```bash
python src/logger_bot/main.py
```

The bot will listen for voice and audio messages. When it receives one, it downloads the file, transcribes it with Whisper, and replies with the transcription.

## Project structure

```
src/logger_bot/
├── main.py     # Bot entry point and Telegram handler
├── model.py    # Whisper transcription and workout data extraction
├── storage.py  # Google Sheets integration (not yet wired in)
```

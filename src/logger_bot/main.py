import os
import logging
from pathlib import Path
from telegram import Update
from telegram.ext import (
    ApplicationBuilder,
    ContextTypes,
    MessageHandler,
    filters,
)
from dotenv import load_dotenv


# Code of your application, which uses environment variables (e.g. from `os.environ` or
# `os.getenv`) as if they came from the actual environment.

from logger_bot.model import Extractor
from logger_bot.logger import SimpleCSVLogger

logging.basicConfig(
    format="%(asctime)s - %(name)s - %(levelname)s - %(message)s", level=logging.INFO
)

extractor = Extractor()
logger = SimpleCSVLogger()


def speech2text(file_path, extractor: Extractor):
    return extractor.transcribe_audio(file_path)


async def handle_message(update: Update, context: ContextTypes.DEFAULT_TYPE):
    audio = update.message.audio or update.message.voice or update.message.document
    if not audio:
        await context.bot.send_message(
            chat_id=update.effective_chat.id, text="No audio file found."
        )
        return
    file = await context.bot.get_file(audio.file_id)
    file_path = f"received_{audio.file_id}"
    await file.download_to_drive(custom_path=file_path)
    text = speech2text(file_path, extractor=extractor)
    # TODO: clean up text
    logger.write_record(text)

    logging.info(f"Escuche {text}")
    await context.bot.send_message(
        chat_id=update.effective_chat.id, text=f"Escuche esto: {text}"
    )


if __name__ == "__main__":
    load_dotenv()
    token = os.environ["TELEGRAM_API_TOKEN"]
    application = ApplicationBuilder().token(token).build()

    handler = MessageHandler(
        filters.TEXT | filters.AUDIO | filters.VOICE | filters.Document.AUDIO,
        handle_message,
    )
    application.add_handler(handler)
    application.run_polling()

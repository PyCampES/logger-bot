import logging
from pathlib import Path
from telegram import Update
from telegram.ext import (
    ApplicationBuilder,
    ContextTypes,
    MessageHandler,
    filters,
)

from logger_bot.model import Extractor

logging.basicConfig(
    format="%(asctime)s - %(name)s - %(levelname)s - %(message)s", level=logging.INFO
)

extractor = Extractor()


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

    logging.info(f"Escuche {text}")
    await context.bot.send_message(
        chat_id=update.effective_chat.id, text=f"Escuche esto: {text}"
    )


if __name__ == "__main__":
    token = Path(".env").read_text().split("=")[1].strip()
    application = ApplicationBuilder().token(token).build()

    handler = MessageHandler(
        filters.TEXT | filters.AUDIO | filters.VOICE | filters.Document.AUDIO,
        handle_message,
    )
    application.add_handler(handler)
    application.run_polling()

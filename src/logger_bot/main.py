import os
import logging
import json

from telegram import Update
from telegram.ext import (
    ApplicationBuilder,
    ContextTypes,
    MessageHandler,
    filters,
)
from dotenv import load_dotenv


from logger_bot.extraction import WhisperTranscriber, parse_text
from logger_bot.logger import SimpleCSVLogger

logging.basicConfig(
    format="%(asctime)s - %(name)s - %(levelname)s - %(message)s", level=logging.INFO
)


def create_handler(transcriber, logger):
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
        text = transcriber.transcribe(file_path)
        entry = parse_text(text)
        logger.write_record(entry)

        logging.info(f"He escuchado esto: {json.dumps(entry)}")
        await context.bot.send_message(
            chat_id=update.effective_chat.id, text=f"Escuche esto: {json.dumps(entry)}"
        )

    handler = MessageHandler(
        filters.TEXT | filters.AUDIO | filters.VOICE | filters.Document.AUDIO,
        handle_message,
    )

    return handler


def main():
    load_dotenv()
    application = ApplicationBuilder().token(os.environ["TELEGRAM_API_TOKEN"]).build()
    handler = create_handler(transcriber=WhisperTranscriber(), logger=SimpleCSVLogger())
    application.add_handler(handler)
    application.run_polling()


if __name__ == "__main__":
    main()

import logging
from pathlib import Path
from telegram import Update
from telegram.ext import (
    ApplicationBuilder,
    ContextTypes,
    MessageHandler,
    filters,
)

logging.basicConfig(
    format="%(asctime)s - %(name)s - %(levelname)s - %(message)s", level=logging.INFO
)


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
    await context.bot.send_message(
        chat_id=update.effective_chat.id,
        text=f"Te escuche y mandaste {len(file_path)} bytes",
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

import os
import logging
import json
import sqlite3

from telegram import Update
from telegram.ext import (
    ApplicationBuilder,
    ContextTypes,
    MessageHandler,
    filters,
    CommandHandler,
)
from dotenv import load_dotenv


from logger_bot.extraction import WhisperTranscriber, parse_text
from logger_bot.logger import SimpleCSVLogger

logging.basicConfig(
    format="%(asctime)s - %(name)s - %(levelname)s - %(message)s", level=logging.INFO
)


def create_audio_handler(transcriber, logger):
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
        (filters.TEXT & ~filters.COMMAND)
        | filters.AUDIO
        | filters.VOICE
        | filters.Document.AUDIO,
        handle_message,
    )

    return handler


def get_last_entry_for_exercise(db_connection: sqlite3.Connection, exercise: str):
    db_connection.row_factory = lambda cursor, row: {
        col[0]: row[i] for i, col in enumerate(cursor.description)
    }
    cur = db_connection.cursor()
    res = cur.execute(
        f"""
        select * from logs
        where exercise like "%{exercise}%"
        order by date desc,time desc
        limit 1
    """
    )
    return res.fetchone()


def create_last_handler(db_connection: sqlite3.Connection):
    async def last(update: Update, context: ContextTypes.DEFAULT_TYPE):
        if not context.args:
            await context.bot.send_message(
                chat_id=update.effective_chat.id, text="No exercise parsed"
            )
            return
        if len(context.args) > 1:
            await context.bot.send_message(
                chat_id=update.effective_chat.id, text="Only 1 arg is tolerated"
            )
            return

        exercise = context.args[0]
        entry = get_last_entry_for_exercise(
            db_connection=db_connection, exercise=exercise
        )
        await context.bot.send_message(
            chat_id=update.effective_chat.id,
            parse_mode="MarkdownV2",
            text=f"""\
    {exercise} Last workout:

    ```
    {json.dumps(entry, indent=2)}
    ```\
    """,
        )

    return CommandHandler("last", last, has_args=True)


def create_sql_handler(db_connection: sqlite3.Connection):
    async def sql(update: Update, context: ContextTypes.DEFAULT_TYPE):
        query = " ".join(context.args) if context.args else ""
        if not query:
            await update.message.reply_text("No query detected")
            return

        db_connection.row_factory = lambda cursor, row: {
            col[0]: row[i] for i, col in enumerate(cursor.description)
        }

        cur = db_connection.cursor()
        rows = cur.execute(query).fetchall()
        text = f"```\n{json.dumps(rows, indent=2)}\n```"
        await context.bot.send_message(
            chat_id=update.effective_chat.id, text=text, parse_mode="MarkdownV2"
        )

    return CommandHandler("sql", sql)


def main():
    load_dotenv()
    application = ApplicationBuilder().token(os.environ["TELEGRAM_API_TOKEN"]).build()
    audio_handler = create_audio_handler(
        transcriber=WhisperTranscriber(), logger=SimpleCSVLogger()
    )
    application.add_handler(audio_handler)

    db_path = "./log.db"
    db_connection = sqlite3.connect(f"file:{db_path}?mode=ro", uri=True)
    last_handler = create_last_handler(db_connection=db_connection)
    application.add_handler(last_handler)

    sql_handler = create_sql_handler(db_connection=db_connection)
    application.add_handler(sql_handler)

    application.run_polling()


if __name__ == "__main__":
    main()

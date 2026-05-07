import json
import os
import sqlite3

from telegram import Update
from telegram.ext import (
    ApplicationBuilder,
    CommandHandler,
    ContextTypes,
    MessageHandler,
    filters,
)
from dotenv import load_dotenv

from logger_bot.extraction import WhisperTranscriber, parse_text
from logger_bot.loggers import SqliteLogger


def create_audio_handler(transcriber, logger):
    async def handle_message(update: Update, context: ContextTypes.DEFAULT_TYPE):
        try:
            audio = (
                update.message.audio or update.message.voice or update.message.document
            )
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

            print(f"I heard: {json.dumps(entry)}")
            await context.bot.send_message(
                chat_id=update.effective_chat.id,
                text=f"I heard:\n{json.dumps(entry)}",
            )
        except Exception as e:
            await context.bot.send_message(
                chat_id=update.effective_chat.id, text=f"Error: {e}"
            )

    handler = MessageHandler(
        (filters.TEXT & ~filters.COMMAND)
        | filters.AUDIO
        | filters.VOICE
        | filters.Document.AUDIO,
        handle_message,
    )

    return handler


def get_last_entry_for_exercise(
    db_conn: sqlite3.Connection, table_name: str, exercise: str
):
    db_conn.row_factory = lambda cursor, row: {
        col[0]: row[i] for i, col in enumerate(cursor.description)
    }
    cur = db_conn.cursor()
    res = cur.execute(
        f"""
        select * from {table_name}
        where exercise like "%{exercise}%"
        order by date desc,time desc
        limit 1
    """
    )
    return res.fetchone()


def create_last_handler(db_conn: sqlite3.Connection, table_name: str):
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

        try:
            exercise = context.args[0]
            entry = get_last_entry_for_exercise(
                db_conn=db_conn, table_name=table_name, exercise=exercise
            )
            await context.bot.send_message(
                chat_id=update.effective_chat.id,
                parse_mode="MarkdownV2",
                text=f"""\
                Last {exercise} workout:\n
                ```
                {json.dumps(entry, indent=2)}
                ```
                """,
            )
        except Exception as e:
            await context.bot.send_message(
                chat_id=update.effective_chat.id, text=f"Error: {e}"
            )

    return CommandHandler("last", last, has_args=True)


def create_sql_handler(db_conn: sqlite3.Connection):
    async def sql(update: Update, context: ContextTypes.DEFAULT_TYPE):
        try:
            query = " ".join(context.args) if context.args else ""
            if not query:
                await update.message.reply_text("No query detected")
                return

            db_conn.row_factory = lambda cursor, row: {
                col[0]: row[i] for i, col in enumerate(cursor.description)
            }

            cur = db_conn.cursor()
            rows = cur.execute(query).fetchall()
            text = f"```\n{json.dumps(rows, indent=2)}\n```"
            await context.bot.send_message(
                chat_id=update.effective_chat.id, text=text, parse_mode="MarkdownV2"
            )
        except Exception as e:
            await context.bot.send_message(
                chat_id=update.effective_chat.id, text=f"Error: {e}"
            )

    return CommandHandler("sql", sql)


async def health(update: Update, context: ContextTypes.DEFAULT_TYPE):
    await update.message.reply_text("Server is running")


def main():
    load_dotenv()
    application = ApplicationBuilder().token(os.environ["TELEGRAM_API_TOKEN"]).build()

    db_path = "./log.db"
    table_name = "workout"
    model_size = os.environ.get("MODEL_SIZE", "base")
    transcriber = WhisperTranscriber(model_size=model_size)
    audio_handler = create_audio_handler(
        transcriber=transcriber, logger=SqliteLogger(filename=db_path)
    )
    application.add_handler(audio_handler)

    db_conn = sqlite3.connect(f"file:{db_path}?mode=ro", uri=True)
    last_handler = create_last_handler(db_conn=db_conn, table_name=table_name)
    application.add_handler(last_handler)

    sql_handler = create_sql_handler(db_conn=db_conn)
    application.add_handler(sql_handler)

    application.add_handler(CommandHandler("health", health))

    print(f"Whisper model size: {model_size}")
    print("Server up, you're ready to go.")
    application.run_polling()


if __name__ == "__main__":
    main()

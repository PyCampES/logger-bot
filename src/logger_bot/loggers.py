import abc

import csv
from datetime import datetime
from pathlib import Path
import sqlite3


class Logger(abc.ABC):
    @abc.abstractmethod
    def write_record(self, data: dict) -> None: ...


class CSVLogger(Logger):
    def __init__(self, filename: str):
        self.filename = filename
        self.headers = [
            "date",
            "time",
            "category",
            "exercise",
            "reps",
            "weight",
            "unit",
            "raw_text",
        ]
        if not Path(self.filename).exists():
            with open(self.filename, "w", newline="") as f:
                writer = csv.writer(f)
                writer.writerow(self.headers)

    def write_record(self, data: dict) -> None:
        now = datetime.now()
        row = [
            now.strftime("%Y-%m-%d"),
            now.strftime("%H:%M:%S"),
            data.get("exercise", ""),
            data.get("category", ""),
            data.get("reps", ""),
            data.get("weight", ""),
            data.get("unit", ""),
            data.get("raw_text", ""),
        ]
        with open(self.filename, "a", newline="") as f:
            writer = csv.writer(f)
            writer.writerow(row)


class SqliteLogger(Logger):
    def __init__(self, filename: str, table_name="workout"):
        self.filename = filename
        self.table_name = table_name
        self.headers = [
            "date",
            "time",
            "category",
            "exercise",
            "reps",
            "weight",
            "unit",
            "raw_text",
        ]
        self._ensure_table()

    def _ensure_table(self):
        with sqlite3.connect(self.filename) as conn:
            c = conn.cursor()
            c.execute(
                f"""
                CREATE TABLE IF NOT EXISTS {self.table_name} (
                    date TEXT,
                    time TEXT,
                    category TEXT,
                    exercise TEXT,
                    reps TEXT,
                    weight TEXT,
                    unit TEXT,
                    raw_text TEXT
                )
            """
            )
            conn.commit()

    def write_record(self, data: dict) -> None:
        now = datetime.now()
        row = [
            now.strftime("%Y-%m-%d"),
            now.strftime("%H:%M:%S"),
            data.get("category", ""),
            data.get("exercise", ""),
            data.get("reps", ""),
            data.get("weight", ""),
            data.get("unit", ""),
            data.get("raw_text", ""),
        ]
        with sqlite3.connect(self.filename) as conn:
            c = conn.cursor()
            c.execute(
                f"""
                INSERT INTO {self.table_name}
                (date, time, category, exercise, reps, weight, unit, raw_text)
                VALUES (?, ?, ?, ?, ?, ?, ?, ?)
            """,
                row,
            )
            conn.commit()

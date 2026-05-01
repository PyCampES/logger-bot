import abc

import csv
from datetime import datetime
from pathlib import Path


class Logger(abc.ABC):
    @abc.abstractmethod
    def write_record(self, data: dict) -> None: ...


class SimpleCSVLogger(Logger):
    def __init__(self, filename: str = "log.csv"):
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
            print("here")
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


#     def get_records(self) -> list[list[str]]:
#         """
#         Returns all rows as a list of lists (each row is a list of strings).
#         """
#         with open(self.filename, "r", newline="") as f:
#             reader = csv.reader(f)
#             return list(reader)

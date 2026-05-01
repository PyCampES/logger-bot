import csv
import os
from datetime import datetime
from typing import Optional
from pathlib import Path


class SimpleCSVLogger:
    def __init__(self, filename: str = "log.csv"):
        self.filename = filename

    def write_record(self, data: str):
        with open(self.filename, "a") as f:
            row = f"{datetime.now()}|{data}\n"
            f.write(row)


# class Logger:
#     def __init__(self, filename: str = "log.csv"):
#         self.filename = filename
#         self.headers = [
#             "date",
#             "time",
#             "exercise",
#             "reps",
#             "weight",
#             "unit",
#             "raw_text",
#         ]
#         # Initialize file if it doesn't exist
#         if not os.path.exists(self.filename):
#             with open(self.filename, "w", newline="") as f:
#                 writer = csv.writer(f)
#                 writer.writerow(self.headers)
#
#     def save_record(self, data: dict[str, Optional[str]]):
#         """
#         Appends a new row to the CSV log. Data keys should include:
#         'exercise', 'reps', 'weight', 'unit', 'raw_text' (all as str).
#         Other keys are ignored.
#         Automatically adds date and time.
#         """

#         now = datetime.now()
#         row = [
#             now.strftime("%Y-%m-%d"),
#             now.strftime("%H:%M:%S"),
#             data.get("exercise", ""),
#             data.get("reps", ""),
#             data.get("weight", ""),
#             data.get("unit", ""),
#             data.get("raw_text", ""),
#         ]
#         with open(self.filename, "a", newline="") as f:
#             writer = csv.writer(f)
#             writer.writerow(row)
#
#     def get_records(self) -> list[list[str]]:
#         """
#         Returns all rows as a list of lists (each row is a list of strings).
#         """
#         with open(self.filename, "r", newline="") as f:
#             reader = csv.reader(f)
#             return list(reader)

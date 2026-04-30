import gspread
from google.oauth2.service_account import Credentials

class Storage:
    def __init__(self, credentials_path="credentials.json", sheet_name="Logueala"):
        self.scopes = [
            "https://www.googleapis.com/auth/spreadsheets",
            "https://www.googleapis.com/auth/drive"
        ]
        self.credentials_path = credentials_path
        self.sheet_name = sheet_name
        self.client = None
        self.sheet = None

    def connect(self):
        try:
            creds = Credentials.from_service_account_file(self.credentials_path, scopes=self.scopes)
            self.client = gspread.authorize(creds)
            # Open the spreadsheet by title (make sure it exists and is shared with the service account email)
            spreadsheet = self.client.open(self.sheet_name)
            self.sheet = spreadsheet.sheet1
            return True
        except Exception as e:
            print(f"Failed to connect to Google Sheets: {e}")
            return False

    def save_record(self, data: dict) -> bool:
        """Save the parsed record to the table."""
        if not self.sheet:
            connected = self.connect()
            if not connected:
                return False
                
        import datetime
        now = datetime.datetime.now()
        date_str = now.strftime("%Y-%m-%d")
        time_str = now.strftime("%H:%M:%S")
        
        row = [
            date_str,
            time_str,
            data.get("exercise", ""),
            data.get("reps", ""),
            data.get("weight", ""),
            data.get("unit", ""),
            data.get("raw_text", "")
        ]
        
        try:
            # Append row to the sheet
            self.sheet.append_row(row)
            return True
        except Exception as e:
            print(f"Failed to append row: {e}")
            return False

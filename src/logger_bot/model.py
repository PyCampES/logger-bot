import whisper
import re

class Extractor:
    def __init__(self):
        # Load the base whisper model (fast and reasonable accuracy)
        print("Loading Whisper model...")
        self.model = whisper.load_model("base")
        print("Whisper model loaded.")
        
    def transcribe_audio(self, file_path: str) -> str:
        """Transcribe the audio file to text."""
        try:
            result = self.model.transcribe(file_path)
            return result["text"].strip()
        except Exception as e:
            print(f"Error during transcription: {e}")
            return ""

    def extract_data(self, text: str) -> dict:
        """Extract exercise, reps, and weight from text."""
        # Clean the text
        text_lower = text.lower()
        
        # 1. Try to find weight first
        weight_match = re.search(r'(\d+(?:\.\d+)?)\s*(kg|kilos?|lbs?|pounds?|libras?)', text_lower)
        weight = float(weight_match.group(1)) if weight_match else 0.0
        unit = weight_match.group(2) if weight_match else "kg"
            
        # Remove weight from text so it's not confused with reps
        if weight_match:
            text_lower = text_lower.replace(weight_match.group(0), "")
            
        # 2. Try to find reps
        # Look for explicit reps first, with longest Spanish words first to avoid partial matches
        reps_match = re.search(r'(\d+)\s*(repeticiones?|repetitions?|reps?)', text_lower)
        if reps_match:
            reps = int(reps_match.group(1))
            text_lower = text_lower.replace(reps_match.group(0), "")
        else:
            # Fallback: If no explicit 'reps' word is used, assume the first number we find is the reps 
            # (e.g. "10 sentadillas")
            any_num = re.search(r'\b(\d+)\b', text_lower)
            if any_num:
                reps = int(any_num.group(1))
                text_lower = text_lower.replace(any_num.group(0), "")
            else:
                reps = 0
                
        # 3. Try to find exercise name (everything else roughly)
        exercise = text_lower
            
        # Clean up punctuation before removing filler words
        exercise = re.sub(r'[^\w\s]', ' ', exercise)
        
        exercise_lower = exercise.lower()
        for wrong, right in spelling_corrections.items():
            # Use word boundaries for exact word replacements
            exercise_lower = re.sub(r'\b' + wrong + r'\b', right, exercise_lower)
            
        exercise = exercise_lower.title()
        
        if not exercise:
            exercise = "Unknown Exercise"
            
        return {
            "exercise": exercise,
            "reps": reps,
            "weight": weight,
            "unit": unit,
            "raw_text": text
        }

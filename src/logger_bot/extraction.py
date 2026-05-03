import whisper
import re
import torch


class WhisperTranscriber:
    def __init__(self, model_size: str = "base"):
        print("Loading Whisper model...")
        if torch.cuda.is_available():
            device = "cuda"
        elif torch.backends.mps.is_available():
            device = "mps"
        else:
            device = "cpu"
        self.model = whisper.load_model(model_size, device=device)
        print("Whisper model loaded.")

    def transcribe(self, file_path: str) -> str:
        """Transcribe the audio file to text."""
        try:
            result = self.model.transcribe(file_path)
            return result["text"].strip()
        except Exception as e:
            print(f"Error during transcription: {e}")
            return ""


def parse_text(text: str) -> dict:
    """Extract category, exercise, reps, and weight from text."""
    text_lower = text.lower()

    # 1. Try to find weight
    weight_match = re.search(
        r"(\d+(?:\.\d+)?)\s*(kg|kilos?|lbs?|pounds?|libras?)", text_lower
    )
    weight = float(weight_match.group(1)) if weight_match else 0.0
    raw_unit = weight_match.group(2) if weight_match else "kg"

    # Normalize unit
    if raw_unit in ("kilo", "kilos"):
        unit = "kg"
    elif raw_unit in ("lb", "lbs", "pound", "pounds", "libra", "libras"):
        unit = "lbs"
    else:
        unit = raw_unit

    # Remove weight + unit from text so it's not confused with reps
    if weight_match:
        text_lower = text_lower.replace(weight_match.group(0), " ")

    # 2. Try to find reps
    reps_match = re.search(r"(\d+)\s*(repeticiones?|repetitions?|reps?)", text_lower)
    if reps_match:
        reps = int(reps_match.group(1))
        text_lower = text_lower.replace(reps_match.group(0), " ")
    else:
        reps = 0

    # 3. Extract category (e.g. "categoria piernas" or "categoría espalda")
    category = ""
    cat_match = re.search(r"categor[ií]a[\s,]+(?:de\s+)?(\w+)", text_lower)
    if cat_match:
        category = cat_match.group(1).title()
        # Remove the full category phrase from text
        text_lower = text_lower.replace(cat_match.group(0), " ")

    # 4. Build exercise name from remaining text
    # Remove noise words/phrases and punctuation
    remaining = re.sub(r"por\s+lado", "", text_lower)
    remaining = re.sub(
        r"(cuatro|cinco|seis|siete|ocho|nueve|diez)\s+repeticiones?", "", remaining
    )
    remaining = re.sub(r"con\b", "", remaining)
    remaining = re.sub(r"[^\w\s]", " ", remaining)
    # Collapse whitespace and strip
    exercise = " ".join(remaining.split()).strip().title()

    if not exercise:
        exercise = "Unknown Exercise"

    return {
        "category": category,
        "exercise": exercise,
        "reps": reps,
        "weight": weight,
        "unit": unit,
        "raw_text": text,
    }

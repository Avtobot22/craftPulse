def title(text):
    if not text:
        return ""
    words = text.split()
    return " ".join(word.capitalize() for word in words)

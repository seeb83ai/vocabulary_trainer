#!/usr/bin/env python3
"""
Generate an MP3 audio file for a Chinese vocabulary word using edge-tts.

Usage:
    python3 generate.py <word_id> <zh_text> <output_dir>

The output file is written to:
    <output_dir>/<word_id>.mp3

Exit codes:
    0  success (or file already exists)
    1  error
"""
import asyncio
import os
import sys


VOICE = "zh-CN-XiaoxiaoNeural"


async def generate(word_id: str, zh_text: str, output_dir: str) -> None:
    try:
        import edge_tts
    except ImportError:
        print("edge-tts not installed. Run: pip install edge-tts", file=sys.stderr)
        sys.exit(1)

    os.makedirs(output_dir, exist_ok=True)
    out_path = os.path.join(output_dir, f"{word_id}.mp3")

    if os.path.exists(out_path):
        return  # already cached

    communicate = edge_tts.Communicate(zh_text, VOICE)
    await communicate.save(out_path)


def main() -> None:
    if len(sys.argv) != 4:
        print(f"Usage: {sys.argv[0]} <word_id> <zh_text> <output_dir>", file=sys.stderr)
        sys.exit(1)

    word_id, zh_text, output_dir = sys.argv[1], sys.argv[2], sys.argv[3]
    asyncio.run(generate(word_id, zh_text, output_dir))


if __name__ == "__main__":
    main()

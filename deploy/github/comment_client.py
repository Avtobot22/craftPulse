#!/usr/bin/env python3
"""Send a JSON comment request to the stand's restricted local bot socket."""

import argparse
import json
import socket
import sys


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--socket", default="/run/craftpulse-github89-comment.sock")
    args = parser.parse_args()
    body = sys.stdin.buffer.read(32769)
    if len(body) > 32768:
        raise ValueError("Request too large")
    with socket.socket(socket.AF_UNIX, socket.SOCK_STREAM) as connection:
        connection.settimeout(65)
        connection.connect(args.socket)
        connection.sendall(body)
        connection.shutdown(socket.SHUT_WR)
        chunks = []
        while True:
            chunk = connection.recv(4096)
            if not chunk:
                break
            chunks.append(chunk)
            if sum(map(len, chunks)) > 4096:
                raise ValueError("Response too large")
    raw = b"".join(chunks)
    if not raw:
        raise ValueError("Request denied or GitHub unavailable; inspect the service journal")
    result = json.loads(raw)
    if set(result) != {"id", "changed"}:
        raise ValueError("Unexpected bot response")
    print(json.dumps(result))


if __name__ == "__main__":
    try:
        main()
    except (ValueError, OSError) as error:
        print(str(error), file=sys.stderr)
        sys.exit(1)

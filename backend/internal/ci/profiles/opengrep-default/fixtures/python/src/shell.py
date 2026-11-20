import subprocess


def archive_unsafe(path):
    subprocess.run("tar czf backup.tgz " + path, shell=True, check=True)


def archive_safe(path):
    subprocess.run(["tar", "czf", "backup.tgz", path], check=True)

package fixture

import "os/exec"

func archiveUnsafe(path string) error {
	return exec.Command("sh", "-c", "tar czf backup.tgz "+path).Run()
}

func archiveSafe(path string) error {
	return exec.Command("tar", "czf", "backup.tgz", path).Run()
}

package dataset

import (
	"bytes"
	"compress/zlib"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Git objects are built in process, so a commit SHA depends only on the model and not on the git version
// or the user's git config.

type gitObjects map[string][]byte

type gitSignature struct {
	name, email string
	at          time.Time
	offset      string
}

func (s gitObjects) put(kind string, body []byte) string {
	object := append(fmt.Appendf(nil, "%s %d\x00", kind, len(body)), body...)
	sum := sha1.Sum(object)
	id := hex.EncodeToString(sum[:])
	s[id] = object
	return id
}

func blobID(content []byte) string {
	return gitObjects{}.put("blob", content)
}

// tree stores the files and every subtree; paths use forward slashes.
func (s gitObjects) tree(files map[string][]byte) string {
	type entry struct {
		name, mode, id string
	}
	subtrees := make(map[string]map[string][]byte)
	var entries []entry
	for path, content := range files {
		dir, rest, nested := strings.Cut(path, "/")
		if !nested {
			entries = append(entries, entry{name: path, mode: "100644", id: s.put("blob", content)})
			continue
		}
		if subtrees[dir] == nil {
			subtrees[dir] = make(map[string][]byte)
		}
		subtrees[dir][rest] = content
	}
	for dir, children := range subtrees {
		entries = append(entries, entry{name: dir, mode: "40000", id: s.tree(children)})
	}
	// Git orders a directory as if its name ended with a slash.
	key := func(e entry) string {
		if e.mode == "40000" {
			return e.name + "/"
		}
		return e.name
	}
	sort.Slice(entries, func(i, j int) bool { return key(entries[i]) < key(entries[j]) })
	var body bytes.Buffer
	for _, e := range entries {
		raw, _ := hex.DecodeString(e.id) // ids come from put and are always valid hex
		fmt.Fprintf(&body, "%s %s\x00", e.mode, e.name)
		body.Write(raw)
	}
	return s.put("tree", body.Bytes())
}

func (s gitObjects) commit(tree string, parents []string, author, committer gitSignature, message string) string {
	var body strings.Builder
	fmt.Fprintf(&body, "tree %s\n", tree)
	for _, parent := range parents {
		fmt.Fprintf(&body, "parent %s\n", parent)
	}
	fmt.Fprintf(&body, "author %s\ncommitter %s\n\n%s\n", author, committer, message)
	return s.put("commit", []byte(body.String()))
}

func (g gitSignature) String() string {
	return fmt.Sprintf("%s <%s> %d %s", g.name, g.email, g.at.Unix(), gitOffset(g.offset))
}

func gitOffset(offset string) string {
	if offset == "Z" {
		return "+0000"
	}
	return strings.ReplaceAll(offset, ":", "")
}

// writeBare writes a bare repository with loose objects and one branch pointing at head.
func (s gitObjects) writeBare(dir, branch, head string) error {
	if _, err := os.Stat(dir); err == nil {
		return fmt.Errorf("repository directory already exists: %s", dir)
	}
	for _, sub := range []string{"objects/info", "objects/pack", "refs/heads", "refs/tags"} {
		if err := os.MkdirAll(filepath.Join(dir, sub), 0o700); err != nil {
			return fmt.Errorf("create repository: %w", err)
		}
	}
	files := map[string]string{
		"HEAD":                 "ref: refs/heads/" + branch + "\n",
		"config":               "[core]\n\trepositoryformatversion = 0\n\tfilemode = true\n\tbare = true\n",
		"refs/heads/" + branch: head + "\n",
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
			return fmt.Errorf("write repository file: %w", err)
		}
	}
	ids := make([]string, 0, len(s))
	for id := range s {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		if err := writeLoose(filepath.Join(dir, "objects", id[:2]), id[2:], s[id]); err != nil {
			return err
		}
	}
	return nil
}

func writeLoose(dir, name string, object []byte) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create object directory: %w", err)
	}
	var packed bytes.Buffer
	writer := zlib.NewWriter(&packed)
	if _, err := writer.Write(object); err != nil {
		return fmt.Errorf("compress object: %w", err)
	}
	if err := writer.Close(); err != nil {
		return fmt.Errorf("compress object: %w", err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), packed.Bytes(), 0o600); err != nil {
		return fmt.Errorf("write object: %w", err)
	}
	return nil
}

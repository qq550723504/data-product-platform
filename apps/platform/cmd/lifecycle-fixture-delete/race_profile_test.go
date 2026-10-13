package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
)

func TestRaceManifestCannotExpandConfirmedVersions(t *testing.T) {
	dir := t.TempDir()
	c := config{OutputDirectory: dir, ProviderInstanceID: uuid.New(), IncarnationID: uuid.New()}
	m := manifest{Objects: []object{
		{Namespace: "dpp-lifecycle307-online", Key: "fixture/a.csv", Version: "0f6aa755-4b86-496b-9ee1-43d7c59a8bb3"},
		{Namespace: "dpp-lifecycle307-online", Key: "fixture/b.csv", Version: "61bed66a-9fe1-47bd-8a97-a376ad7bed89"},
		{Namespace: "dpp-lifecycle307-quarantine", Key: "fixture/a.csv", Version: "a0a94326-1914-44e0-aaf8-577b6d824084"},
		{Namespace: "dpp-lifecycle307-quarantine", Key: "fixture/b.csv", Version: "e1611b0a-efc0-4939-abc2-30394ec2c8d2"},
	}}
	for i := range m.Objects {
		m.Objects[i].CopyID = uuid.New()
		m.Objects[i].DatasetVersionID = uuid.New()
		m.Objects[i].SHA256 = "722b7c8834f35892e3a3404b43ff8118b326b5c0c992207290dcc674f6b7ade0"
		m.Objects[i].Bytes = 232
	}
	save := func() {
		b, _ := json.Marshal(m)
		if e := os.WriteFile(filepath.Join(dir, "manifest.json"), b, 0600); e != nil {
			t.Fatal(e)
		}
	}
	save()
	if _, e := raceCopies(c); e != nil {
		t.Fatal(e)
	}
	prior := m.Objects[3]
	for _, change := range []func(){func() { m.Objects[3].Version = "ce87f789-a4f4-4ce7-bf1f-a044ac53d1d2" }, func() { m.Objects[3] = m.Objects[2] }, func() { m.Objects[3].Namespace = "other" }, func() { m.Objects = m.Objects[:3] }} {
		m.Objects = m.Objects[:4]
		m.Objects[3] = prior
		change()
		save()
		if _, e := raceCopies(c); e == nil {
			t.Fatal("foreign, duplicate, missing or consumed version admitted")
		}
	}
}

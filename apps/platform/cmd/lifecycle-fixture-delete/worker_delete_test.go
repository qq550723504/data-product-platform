package main

import (
	"encoding/json"
	"github.com/google/uuid"
	"os"
	"path/filepath"
	"testing"
)

func TestWorkerDeleteManifestRejectsRetargeting(t *testing.T) {
	dir := t.TempDir()
	c := config{OutputDirectory: dir, ProviderInstanceID: uuid.New(), IncarnationID: uuid.New()}
	m := manifest{Objects: []object{
		{Namespace: "dpp-lifecycle307-online", Key: "fixture/a.csv", Version: "4a225512-f6ac-4fc9-b0c0-bd6e016a2a01"},
		{Namespace: "dpp-lifecycle307-online", Key: "fixture/b.csv", Version: "7cd23e55-140c-4566-b881-e58c423c7a8a"},
		{Namespace: "dpp-lifecycle307-quarantine", Key: "fixture/a.csv", Version: "936c8741-87f6-4279-87b9-742936cb2dd2"},
		{Namespace: "dpp-lifecycle307-quarantine", Key: "fixture/b.csv", Version: "ce87f789-a4f4-4ce7-bf1f-a044ac53d1d2"},
	}}
	for i := range m.Objects {
		m.Objects[i].CopyID = uuid.New()
		m.Objects[i].DatasetVersionID = uuid.New()
		m.Objects[i].SHA256 = "aafd9ae024f2444eefea042c59ffd87c29fac3f4675c49124785b417da34b784"
		m.Objects[i].Bytes = 26
	}
	save := func() {
		t.Helper()
		b, _ := json.Marshal(m)
		if e := os.WriteFile(filepath.Join(dir, "manifest.json"), b, 0600); e != nil {
			t.Fatal(e)
		}
	}
	save()
	if _, e := workerCopies(c); e != nil {
		t.Fatal(e)
	}
	original := m.Objects[3]
	for _, mutation := range []func(){
		func() { m.Objects[3].Version = "3fe1cc44-b0f8-428c-9bb8-9f92dac5d15d" },
		func() { m.Objects[3].Key = "fixture/a.csv" },
		func() { m.Objects[3] = m.Objects[2] },
		func() { m.Objects[3].Namespace = "foreign" },
	} {
		m.Objects[3] = original
		mutation()
		save()
		if _, e := workerCopies(c); e == nil {
			t.Fatal("retargeted/duplicate scope accepted")
		}
	}
}

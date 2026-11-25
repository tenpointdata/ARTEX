package db

import "testing"

// TestProfileSamplingRoundTrip pins the sampling-pin columns (migration 0027)
// through both read column lists and the keep-key update branch, and checks
// that clearing the pin stores NULL again (not a zero seed).
func TestProfileSamplingRoundTrip(t *testing.T) {
	d, err := Open(testDSN(t))
	if err != nil {
		SkipPGUnavailable(t, err)
	}
	t.Cleanup(func() { d.Close() })

	seed, temp := int64(1234), 0.3
	id, err := d.SaveProfile(&LLMProfile{Name: "t-sampling", Format: "openai", Model: "m", APIKey: "k",
		Sampling: Sampling{Seed: &seed, Temperature: &temp}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Exec(`DELETE FROM llm_profiles WHERE id=$1`, id) })

	p, err := d.ProfileByID(id)
	if err != nil || p == nil {
		t.Fatalf("ProfileByID: %v", err)
	}
	if p.Sampling.Seed == nil || *p.Sampling.Seed != seed || p.Sampling.Temperature == nil || *p.Sampling.Temperature != temp {
		t.Fatalf("keyed load: %+v", p.Sampling)
	}
	ps, err := d.ListProfiles()
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, x := range ps {
		if x.ID == id {
			found = x.Sampling.Seed != nil && *x.Sampling.Seed == seed
		}
	}
	if !found {
		t.Fatal("list load lost the seed")
	}

	p.APIKey = ""
	p.Sampling = Sampling{}
	if _, err := d.SaveProfile(p); err != nil {
		t.Fatal(err)
	}
	after, err := d.ProfileByID(id)
	if err != nil || after == nil {
		t.Fatalf("reload: %v", err)
	}
	if after.Sampling.Seed != nil || after.Sampling.Temperature != nil {
		t.Fatalf("cleared pin read back as %+v", after.Sampling)
	}

	bad := 3.0
	p.Sampling = Sampling{Temperature: &bad}
	if _, err := d.SaveProfile(p); err == nil {
		t.Fatal("temperature above 2 passed the CHECK constraint")
	}
}

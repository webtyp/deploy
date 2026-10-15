package tests

import (
	"errors"
	"strings"
	"testing"

	"webtyp.com/deploy"
	"webtyp.com/lang"
)

type FakeDest map[string]string

func (f FakeDest) Has(key string) (bool, error) {
	_, ok := f[key]
	return ok, nil
}
func (f FakeDest) Put(key, value string) error {
	f[key] = value
	return nil
}

type FakeTarget struct {
	id      deploy.TargetID
	runtime FakeDest
}

func (t *FakeTarget) ID() deploy.TargetID { return t.id }
func (t *FakeTarget) Check(p deploy.Plan, ci deploy.Destination) ([]deploy.Missing, error) {
	var m []deploy.Missing
	has, _ := ci.Has("TARGET_CI_SECRET")
	if !has {
		m = append(m, deploy.Missing{Item: "Target CI Secret", Fix: deploy.FixAutomatic})
	}
	return m, nil
}
func (t *FakeTarget) Provision(p deploy.Plan, ci deploy.Destination) error {
	return ci.Put("TARGET_CI_SECRET", "auto")
}
func (t *FakeTarget) Rotate(p deploy.Plan, ci deploy.Destination) error {
	return ci.Put("TARGET_CI_SECRET", "rotated")
}
func (t *FakeTarget) Runtime(p deploy.Plan) deploy.Destination { return t.runtime }
func (t *FakeTarget) Job(p deploy.Plan) deploy.Job             { return deploy.Job{} }

type FakeCI struct {
	id      deploy.CIID
	secrets FakeDest
	runs    int
}

func (c *FakeCI) ID() deploy.CIID { return c.id }
func (c *FakeCI) Check(p deploy.Plan, job deploy.Job) ([]deploy.Missing, error) {
	var m []deploy.Missing
	has, _ := c.secrets.Has("CI_SECRET")
	if !has {
		m = append(m, deploy.Missing{Item: "CI Secret", Fix: deploy.FixAutomatic})
	}
	return m, nil
}
func (c *FakeCI) Provision(p deploy.Plan, job deploy.Job) error {
	return c.secrets.Put("CI_SECRET", "auto")
}
func (c *FakeCI) Secrets(p deploy.Plan) deploy.Destination { return c.secrets }
func (c *FakeCI) Release(p deploy.Plan) (deploy.Run, error) {
	c.runs++
	return deploy.Run{ID: "run1"}, nil
}
func (c *FakeCI) Status(r deploy.Run) (deploy.RunState, error) { return deploy.RunSucceeded, nil }

func validPlan() deploy.Plan {
	return deploy.Plan{
		Name:   "valid",
		Target: "fake.Target",
		CI:     "fake.CI",
	}
}

func defaultDeps() deploy.Deployment {
	return deploy.Deployment{
		Plan:   validPlan(),
		Target: &FakeTarget{id: "fake.Target", runtime: make(FakeDest)},
		CI:     &FakeCI{id: "fake.CI", secrets: make(FakeDest)},
		Local:  make(FakeDest),
	}
}

func TestScanS1(t *testing.T) {
	p, err := deploy.Scan("testdata/S1")
	if err != nil {
		t.Fatal(err)
	}
	if p.Name != "misitio" || p.Domain != "misitio.velty.cl" || p.Target != "webtyp.com/goflare.Worker" || p.CI != "webtyp.com/github.Actions" {
		t.Errorf("Unexpected plan fields: %+v", p)
	}
	if len(p.Databases) != 1 || p.Databases[0].Binding != "DB" {
		t.Error("Databases mismatch")
	}
	if len(p.Buckets) != 1 || p.Buckets[0].Binding != "ASSETS" {
		t.Error("Buckets mismatch")
	}
	if len(p.Vars) != 1 || p.Vars[0].Key != "IAM_BASE_URL" {
		t.Error("Vars mismatch")
	}
	if len(p.Secrets) != 1 || p.Secrets[0].Key != "STRIPE_KEY" {
		t.Error("Secrets mismatch")
	}
}

func TestScanS2(t *testing.T) {
	_, err := deploy.Scan("testdata/S2")
	if !errors.Is(err, deploy.ErrNoPlan) {
		t.Errorf("Expected ErrNoPlan, got %v", err)
	}
}

func TestScanS3(t *testing.T) {
	_, err := deploy.Scan("testdata/S3")
	if err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Errorf("Expected unknown field error, got %v", err)
	}
}

func TestScanS4(t *testing.T) {
	_, err := deploy.Scan("testdata/S4")
	if !errors.Is(err, deploy.ErrMissingBuildTag) {
		t.Errorf("Expected ErrMissingBuildTag, got %v", err)
	}
}

func TestValidationErrors(t *testing.T) {
	tests := []struct {
		name string
		mut  func(*deploy.Plan)
		err  error
	}{
		{"V1_NoName", func(p *deploy.Plan) { p.Name = "" }, deploy.ErrNoName},
		{"V2_NoTarget", func(p *deploy.Plan) { p.Target = "" }, deploy.ErrNoTarget},
		{"V3_NoCI", func(p *deploy.Plan) { p.CI = "" }, deploy.ErrNoCI},
		{"V4_IncompleteDB", func(p *deploy.Plan) { p.Databases = append(p.Databases, deploy.Database{}) }, deploy.ErrIncompleteDatabase},
		{"V5_IncompleteBucket", func(p *deploy.Plan) { p.Buckets = append(p.Buckets, deploy.Bucket{}) }, deploy.ErrIncompleteBucket},
		{"V6_DuplicateSecret", func(p *deploy.Plan) {
			p.Secrets = []deploy.Secret{{Key: "A", What: "A", Source: deploy.Random, To: []deploy.Place{deploy.ToCI}}, {Key: "A", What: "A2", Source: deploy.Random, To: []deploy.Place{deploy.ToCI}}}
		}, deploy.ErrDuplicateSecret},
		{"V7_InvalidSource", func(p *deploy.Plan) {
			p.Secrets = []deploy.Secret{{Key: "A", What: "A", Source: 0, To: []deploy.Place{deploy.ToCI}}}
		}, deploy.ErrInvalidSource},
		{"V8_NoPlace", func(p *deploy.Plan) {
			p.Secrets = []deploy.Secret{{Key: "A", What: "A", Source: deploy.Random, To: nil}}
		}, deploy.ErrNoPlace},
		{"V9_DuplicatePlace", func(p *deploy.Plan) {
			p.Secrets = []deploy.Secret{{Key: "A", What: "A", Source: deploy.Random, To: []deploy.Place{deploy.ToCI, deploy.ToCI}}}
		}, deploy.ErrInvalidPlace},
		{"V10_IncompleteVar", func(p *deploy.Plan) { p.Vars = []deploy.Var{{Key: "A"}} }, deploy.ErrIncompleteVar},
		{"V11_DuplicateVar", func(p *deploy.Plan) { p.Vars = []deploy.Var{{Key: "A", Value: "1"}, {Key: "A", Value: "2"}} }, deploy.ErrDuplicateVar},
		{"V12_VarSecretConflict", func(p *deploy.Plan) {
			p.Vars = []deploy.Var{{Key: "A", Value: "1"}}
			p.Secrets = []deploy.Secret{{Key: "A", What: "A", Source: deploy.Random, To: []deploy.Place{deploy.ToCI}}}
		}, deploy.ErrDuplicateVar},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := validPlan()
			tt.mut(&p)
			d := deploy.Deployment{Plan: p, Target: &FakeTarget{id: "fake.Target"}, CI: &FakeCI{id: "fake.CI"}, Local: make(FakeDest)}
			_, err := d.Check()
			if !errors.Is(err, tt.err) {
				t.Errorf("Expected %v, got %v", tt.err, err)
			}
		})
	}
}

func TestC1_OrderAndC2_C4(t *testing.T) {
	d := defaultDeps()
	d.Plan.Secrets = []deploy.Secret{
		{Key: "R1", What: "R1", Source: deploy.Random, To: []deploy.Place{deploy.ToRuntime}},
		{Key: "T1", What: "T1", Source: deploy.Typed, Guide: []lang.Text{"G1"}, To: []deploy.Place{deploy.ToLocal}},
	}
	m, err := d.Check()
	if err != nil {
		t.Fatal(err)
	}
	if len(m) != 4 {
		t.Fatalf("Expected 4 missing items, got %d", len(m))
	}
	if string(m[0].Item) != "CI Secret" {
		t.Errorf("Expected CI missing first")
	}
	if string(m[1].Item) != "Target CI Secret" {
		t.Errorf("Expected Target missing second")
	}
	if string(m[2].Item) != "R1" || m[2].Fix != deploy.FixAutomatic {
		t.Errorf("Expected R1 with FixAutomatic")
	}
	if string(m[3].Item) != "T1" || m[3].Fix != deploy.FixInput {
		t.Errorf("Expected T1 with FixInput")
	}

	err = m[3].Accept("typed-val")
	if err != nil {
		t.Fatal(err)
	}
	has, _ := d.Local.Has("T1")
	if !has {
		t.Error("Accept did not put value in destination")
	}
}

func TestC5(t *testing.T) {
	d := defaultDeps()
	d.Plan.Secrets = []deploy.Secret{
		{Key: "S1", What: "S1", Source: deploy.Random, To: []deploy.Place{deploy.ToRuntime, deploy.ToLocal}},
	}
	m, _ := d.Check()
	c := 0
	for _, x := range m {
		if string(x.Item) == "S1" {
			c++
		}
	}
	if c != 2 {
		t.Errorf("Expected 2 Missing items for S1 (one per place), got %d", c)
	}
}

func TestProvision(t *testing.T) {
	d := defaultDeps()
	d.Plan.Secrets = []deploy.Secret{
		{Key: "R1", What: "R1", Source: deploy.Random, To: []deploy.Place{deploy.ToRuntime}},
	}
	err := d.Provision()
	if err != nil {
		t.Fatal(err)
	}
	rt := d.Target.(*FakeTarget).runtime
	if len(rt["R1"]) != 43 {
		t.Errorf("Expected 43 chars base64url, got %s", rt["R1"])
	}

	// Ensure never overwrites
	rt["R1"] = "custom"
	d.Provision()
	if rt["R1"] != "custom" {
		t.Errorf("Provision overwrote existing secret")
	}
}

func TestRotate(t *testing.T) {
	d := defaultDeps()
	d.Plan.Secrets = []deploy.Secret{
		{Key: "R1", What: "R1", Source: deploy.Random, To: []deploy.Place{deploy.ToRuntime}},
		{Key: "T1", What: "T1", Source: deploy.Typed, Guide: []lang.Text{"G1"}, To: []deploy.Place{deploy.ToLocal}},
	}
	d.Target.(*FakeTarget).runtime["R1"] = "old"

	m, err := d.Rotate()
	if err != nil {
		t.Fatal(err)
	}
	if d.Target.(*FakeTarget).runtime["R1"] == "old" {
		t.Errorf("Rotate did not overwrite R1")
	}
	if len(m) != 1 || string(m[0].Item) != "T1" {
		t.Errorf("Rotate did not return missing input for T1")
	}
}

func TestRelease(t *testing.T) {
	d := defaultDeps()

	// R1: Check has Missing
	_, err := d.Release()
	if !errors.Is(err, deploy.ErrNotReady) {
		t.Errorf("Expected ErrNotReady, got %v", err)
	}
	if d.CI.(*FakeCI).runs != 0 {
		t.Errorf("CI.Release called when not ready")
	}

	// Provision to satisfy all
	d.Provision()

	// R2: Check clean
	r, err := d.Release()
	if err != nil {
		t.Fatal(err)
	}
	if r.ID != "run1" {
		t.Errorf("Expected run ID run1, got %s", r.ID)
	}
	if d.CI.(*FakeCI).runs != 1 {
		t.Errorf("CI.Release not called")
	}
}

func TestNilTool(t *testing.T) {
	d := defaultDeps()
	d.Target = nil
	_, err := d.Check()
	if !errors.Is(err, deploy.ErrNilTool) {
		t.Errorf("Expected ErrNilTool, got %v", err)
	}
}

func TestLang(t *testing.T) {
	lang.Load(deploy.Dictionary())
	lang.OutLang("es")
	msg := lang.Translate("The CI cannot deploy without it").String()
	if msg != "La integración continua no puede desplegar sin él" {
		t.Errorf("Translation failed, got: %s", msg)
	}
}

func TestLocalMissing(t *testing.T) {
	// LM1
	p := validPlan()
	p.Secrets = []deploy.Secret{
		{Key: "S1", What: "S1", Source: deploy.Random, To: []deploy.Place{deploy.ToRuntime, deploy.ToLocal}},
	}
	local := make(FakeDest)
	m, err := deploy.LocalMissing(p, local)
	if err != nil {
		t.Fatal(err)
	}
	if len(m) != 1 {
		t.Errorf("Expected 1 missing for local, got %d", len(m))
	}

	// LM2
	_, err = deploy.LocalMissing(p, nil)
	if !errors.Is(err, deploy.ErrNilTool) {
		t.Errorf("Expected ErrNilTool, got %v", err)
	}
}

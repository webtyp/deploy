package deploy

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"strings"

	"webtyp.com/lang"
)

// randomSize is the number of random bytes in a generated secret.
const randomSize = 32

// Deployment is a Plan with the tools that carry it out. Local is the
// person's project keyring (ToLocal secrets); app builds it.
type Deployment struct {
	Plan   Plan
	Target Target
	CI     CI
	Local  Destination
}

// validate checks that the Plan is well-formed.
func validate(p Plan) error {
	if p.Name == "" {
		return ErrNoName
	}
	if p.Target == "" {
		return ErrNoTarget
	}
	if p.CI == "" {
		return ErrNoCI
	}
	for _, db := range p.Databases {
		if db.Binding == "" || db.Name == "" {
			return fmt.Errorf("%q: %w", db.Binding+db.Name, ErrIncompleteDatabase)
		}
	}
	for _, b := range p.Buckets {
		if b.Binding == "" || b.Name == "" {
			return fmt.Errorf("%q: %w", b.Binding+b.Name, ErrIncompleteBucket)
		}
	}

	keys := make(map[string]bool)
	for _, s := range p.Secrets {
		if s.Key == "" || keys[s.Key] {
			return fmt.Errorf("%q: %w", s.Key, ErrDuplicateSecret)
		}
		keys[s.Key] = true
		if s.What == "" {
			return fmt.Errorf("%s: %w", s.Key, ErrNoWhat)
		}
		if s.Source != Typed && s.Source != Random {
			return fmt.Errorf("%s: %w", s.Key, ErrInvalidSource)
		}
		if len(s.To) == 0 {
			return fmt.Errorf("%s: %w", s.Key, ErrNoPlace)
		}
		places := make(map[Place]bool)
		for _, place := range s.To {
			if place != ToRuntime && place != ToCI && place != ToLocal || places[place] {
				return fmt.Errorf("%s: %w", s.Key, ErrInvalidPlace)
			}
			places[place] = true
		}
		if s.Source == Typed && len(s.Guide) == 0 {
			return fmt.Errorf("%s: %w", s.Key, ErrNoGuide)
		}
	}

	for _, v := range p.Vars {
		if v.Key == "" || v.Value == "" {
			return fmt.Errorf("%q: %w", v.Key, ErrIncompleteVar)
		}
		if keys[v.Key] {
			return fmt.Errorf("%s: %w", v.Key, ErrDuplicateVar)
		}
		keys[v.Key] = true
	}
	return nil
}

// ready validates the plan and the tools: none nil, each matching the plan.
func (d Deployment) ready() error {
	if err := validate(d.Plan); err != nil {
		return err
	}
	if d.Target == nil || d.CI == nil || d.Local == nil {
		return ErrNilTool
	}
	if d.Target.ID() != d.Plan.Target {
		return fmt.Errorf("%s != %s: %w", d.Target.ID(), d.Plan.Target, ErrTargetMismatch)
	}
	if d.CI.ID() != d.Plan.CI {
		return fmt.Errorf("%s != %s: %w", d.CI.ID(), d.Plan.CI, ErrCIMismatch)
	}
	return nil
}

// at returns the destination of a place and the text saying why it needs the secret.
func (d Deployment) at(place Place) (Destination, lang.Text) {
	switch place {
	case ToRuntime:
		return d.Target.Runtime(d.Plan), textWhyRuntime
	case ToCI:
		return d.CI.Secrets(d.Plan), textWhyCI
	default:
		return d.Local, textWhyLocal
	}
}

// inputMissing is the FixInput item whose Accept puts the typed value into dest.
func inputMissing(s Secret, dest Destination, why lang.Text) Missing {
	key := s.Key
	return Missing{
		Item:   s.What,
		Why:    why,
		Steps:  s.Guide,
		Fix:    FixInput,
		Secret: true,
		Accept: func(v string) error { return dest.Put(key, v) },
	}
}

// absent appends to all the Missing item of s at dest when dest lacks it.
func absent(all []Missing, s Secret, dest Destination, why lang.Text) ([]Missing, error) {
	has, err := dest.Has(s.Key)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", s.Key, err)
	}
	if has {
		return all, nil
	}
	if s.Source == Random {
		return append(all, Missing{Item: s.What, Why: why, Fix: FixAutomatic}), nil
	}
	return append(all, inputMissing(s, dest, why)), nil
}

// Check reports what the deployment still needs: the CI's items, then the
// Target's, then every declared secret absent at a runtime or CI place, then
// the ToLocal items of LocalMissing.
func (d Deployment) Check() ([]Missing, error) {
	if err := d.ready(); err != nil {
		return nil, err
	}

	all, err := d.CI.Check(d.Plan, d.Target.Job(d.Plan))
	if err != nil {
		return nil, err
	}
	targetMissing, err := d.Target.Check(d.Plan, d.CI.Secrets(d.Plan))
	if err != nil {
		return nil, err
	}
	all = append(all, targetMissing...)

	for _, s := range d.Plan.Secrets {
		for _, place := range s.To {
			if place == ToLocal {
				continue
			}
			dest, why := d.at(place)
			if all, err = absent(all, s, dest, why); err != nil {
				return nil, err
			}
		}
	}

	localMissing, err := LocalMissing(d.Plan, d.Local)
	if err != nil {
		return nil, err
	}
	return append(all, localMissing...), nil
}

// LocalMissing returns exactly the Missing items of the ToLocal places. It
// never touches a Target or CI; a nil local is ErrNilTool.
func LocalMissing(p Plan, local Destination) ([]Missing, error) {
	if err := validate(p); err != nil {
		return nil, err
	}
	if local == nil {
		return nil, ErrNilTool
	}
	var all []Missing
	var err error
	for _, s := range p.Secrets {
		for _, place := range s.To {
			if place != ToLocal {
				continue
			}
			if all, err = absent(all, s, local, textWhyLocal); err != nil {
				return nil, err
			}
		}
	}
	return all, nil
}

// randomSecret is 32 bytes from crypto/rand, base64url without padding.
func randomSecret() string {
	b := make([]byte, randomSize)
	rand.Read(b) // never fails since Go 1.24: it crashes the program instead
	return base64.RawURLEncoding.EncodeToString(b)
}

// Provision lets the Target and the CI provision themselves, then generates
// every absent Random secret and puts it at its places. A present secret is
// never touched.
func (d Deployment) Provision() error {
	if err := d.ready(); err != nil {
		return err
	}
	if err := d.Target.Provision(d.Plan, d.CI.Secrets(d.Plan)); err != nil {
		return err
	}
	if err := d.CI.Provision(d.Plan, d.Target.Job(d.Plan)); err != nil {
		return err
	}

	for _, s := range d.Plan.Secrets {
		if s.Source != Random {
			continue
		}
		value := ""
		for _, place := range s.To {
			dest, _ := d.at(place)
			has, err := dest.Has(s.Key)
			if err != nil {
				return fmt.Errorf("%s: %w", s.Key, err)
			}
			if has {
				continue
			}
			if value == "" {
				value = randomSecret()
			}
			if err := dest.Put(s.Key, value); err != nil {
				return fmt.Errorf("%s: %w", s.Key, err)
			}
		}
	}
	return nil
}

// Rotate lets the Target rotate its own secrets, regenerates every Random
// secret at all its places, and returns one FixInput item per Typed secret
// and place, whose Accept overwrites it.
func (d Deployment) Rotate() ([]Missing, error) {
	if err := d.ready(); err != nil {
		return nil, err
	}
	if err := d.Target.Rotate(d.Plan, d.CI.Secrets(d.Plan)); err != nil {
		return nil, err
	}

	var typed []Missing
	for _, s := range d.Plan.Secrets {
		value := ""
		if s.Source == Random {
			value = randomSecret()
		}
		for _, place := range s.To {
			dest, why := d.at(place)
			if s.Source == Typed {
				typed = append(typed, inputMissing(s, dest, why))
				continue
			}
			if err := dest.Put(s.Key, value); err != nil {
				return nil, fmt.Errorf("%s: %w", s.Key, err)
			}
		}
	}
	return typed, nil
}

// Release runs Check; any Missing is ErrNotReady naming the items, else the
// CI releases the plan.
func (d Deployment) Release() (Run, error) {
	missing, err := d.Check()
	if err != nil {
		return Run{}, err
	}
	if len(missing) > 0 {
		items := make([]string, 0, len(missing))
		for _, m := range missing {
			items = append(items, string(m.Item))
		}
		return Run{}, fmt.Errorf("%w: %s", ErrNotReady, strings.Join(items, ", "))
	}
	return d.CI.Release(d.Plan)
}

// Status returns the state of a CI run.
func (d Deployment) Status(r Run) (RunState, error) {
	if err := d.ready(); err != nil {
		return 0, err
	}
	return d.CI.Status(r)
}

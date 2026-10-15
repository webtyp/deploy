package deploy

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"strings"

	"webtyp.com/lang"
)

// Deployment is a Plan with the tools that carry it out. Local is the
// person's project keyring (ToLocal secrets); app builds it.
type Deployment struct {
	Plan   Plan
	Target Target
	CI     CI
	Local  Destination
}

// validate checks if the Plan is well-formed.
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
			return ErrIncompleteDatabase
		}
	}
	for _, b := range p.Buckets {
		if b.Binding == "" || b.Name == "" {
			return ErrIncompleteBucket
		}
	}

	keys := make(map[string]bool)
	for _, s := range p.Secrets {
		if s.Key == "" {
			return ErrDuplicateSecret
		}
		if keys[s.Key] {
			return ErrDuplicateSecret
		}
		keys[s.Key] = true
		if s.What == "" {
			return ErrNoWhat
		}
		if s.Source != Typed && s.Source != Random {
			return ErrInvalidSource
		}
		if len(s.To) == 0 {
			return ErrNoPlace
		}
		places := make(map[Place]bool)
		for _, place := range s.To {
			if place != ToRuntime && place != ToCI && place != ToLocal {
				return ErrInvalidPlace
			}
			if places[place] {
				return ErrInvalidPlace
			}
			places[place] = true
		}
		if s.Source == Typed && len(s.Guide) == 0 {
			return ErrNoGuide
		}
	}

	for _, v := range p.Vars {
		if v.Key == "" || v.Value == "" {
			return ErrIncompleteVar
		}
		if keys[v.Key] {
			return ErrDuplicateVar
		}
		keys[v.Key] = true
	}

	return nil
}

// checkTools verifies tools match the plan and are not nil.
func (d Deployment) checkTools() error {
	if d.Target == nil || d.CI == nil || d.Local == nil {
		return ErrNilTool
	}
	if d.Target.ID() != d.Plan.Target {
		return ErrTargetMismatch
	}
	if d.CI.ID() != d.Plan.CI {
		return ErrCIMismatch
	}
	return nil
}

// Check reports what the deployment still needs.
func (d Deployment) Check() ([]Missing, error) {
	if err := validate(d.Plan); err != nil {
		return nil, err
	}
	if err := d.checkTools(); err != nil {
		return nil, err
	}

	var all []Missing

	ciMissing, err := d.CI.Check(d.Plan, d.Target.Job(d.Plan))
	if err != nil {
		return nil, err
	}
	all = append(all, ciMissing...)

	targetMissing, err := d.Target.Check(d.Plan, d.CI.Secrets(d.Plan))
	if err != nil {
		return nil, err
	}
	all = append(all, targetMissing...)

	for _, s := range d.Plan.Secrets {
		for _, place := range s.To {
			var dest Destination
			var why lang.Text
			if place == ToRuntime {
				dest = d.Target.Runtime(d.Plan)
				why = textWhyRuntime
			} else if place == ToCI {
				dest = d.CI.Secrets(d.Plan)
				why = textWhyCI
			} else if place == ToLocal {
				continue
			}

			has, err := dest.Has(s.Key)
			if err != nil {
				return nil, err
			}
			if !has {
				fix := FixInput
				var accept func(string) error
				if s.Source == Random {
					fix = FixAutomatic
				} else {
					// Need to capture dest and key for Accept
					d := dest
					k := s.Key
					accept = func(v string) error { return d.Put(k, v) }
				}
				all = append(all, Missing{
					Item:   s.What,
					Why:    why,
					Steps:  s.Guide,
					Fix:    fix,
					Secret: true,
					Accept: accept,
				})
			}
		}
	}

	localMissing, err := LocalMissing(d.Plan, d.Local)
	if err != nil {
		return nil, err
	}
	all = append(all, localMissing...)

	return all, nil
}

// LocalMissing returns exactly the Missing items of the ToLocal places.
func LocalMissing(p Plan, local Destination) ([]Missing, error) {
	if err := validate(p); err != nil {
		return nil, err
	}
	if local == nil {
		return nil, ErrNilTool
	}

	var all []Missing
	for _, s := range p.Secrets {
		for _, place := range s.To {
			if place == ToLocal {
				has, err := local.Has(s.Key)
				if err != nil {
					return nil, err
				}
				if !has {
					fix := FixInput
					var accept func(string) error
					if s.Source == Random {
						fix = FixAutomatic
					} else {
						k := s.Key
						accept = func(v string) error { return local.Put(k, v) }
					}
					all = append(all, Missing{
						Item:   s.What,
						Why:    textWhyLocal,
						Steps:  s.Guide,
						Fix:    fix,
						Secret: true,
						Accept: accept,
					})
				}
			}
		}
	}
	return all, nil
}

func generateRandomSecret() string {
	b := make([]byte, 32)
	rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

// Provision creates what is missing and delivers the target's own CI secrets to ci.
func (d Deployment) Provision() error {
	if err := validate(d.Plan); err != nil {
		return err
	}
	if err := d.checkTools(); err != nil {
		return err
	}

	if err := d.Target.Provision(d.Plan, d.CI.Secrets(d.Plan)); err != nil {
		return err
	}
	if err := d.CI.Provision(d.Plan, d.Target.Job(d.Plan)); err != nil {
		return err
	}

	for _, s := range d.Plan.Secrets {
		if s.Source == Random {
			// Generate once, put in all absent places
			var value string
			for _, place := range s.To {
				var dest Destination
				if place == ToRuntime {
					dest = d.Target.Runtime(d.Plan)
				} else if place == ToCI {
					dest = d.CI.Secrets(d.Plan)
				} else if place == ToLocal {
					dest = d.Local
				}

				has, err := dest.Has(s.Key)
				if err != nil {
					return err
				}
				if !has {
					if value == "" {
						value = generateRandomSecret()
					}
					if err := dest.Put(s.Key, value); err != nil {
						return err
					}
				}
			}
		}
	}
	return nil
}

// Rotate replaces every production secret the target owns.
func (d Deployment) Rotate() ([]Missing, error) {
	if err := validate(d.Plan); err != nil {
		return nil, err
	}
	if err := d.checkTools(); err != nil {
		return nil, err
	}

	if err := d.Target.Rotate(d.Plan, d.CI.Secrets(d.Plan)); err != nil {
		return nil, err
	}

	var inputMissing []Missing

	for _, s := range d.Plan.Secrets {
		var value string
		if s.Source == Random {
			value = generateRandomSecret()
		}

		for _, place := range s.To {
			var dest Destination
			var why lang.Text
			if place == ToRuntime {
				dest = d.Target.Runtime(d.Plan)
				why = textWhyRuntime
			} else if place == ToCI {
				dest = d.CI.Secrets(d.Plan)
				why = textWhyCI
			} else if place == ToLocal {
				dest = d.Local
				why = textWhyLocal
			}

			if s.Source == Random {
				if err := dest.Put(s.Key, value); err != nil {
					return nil, err
				}
			} else if s.Source == Typed {
				d2 := dest
				k := s.Key
				inputMissing = append(inputMissing, Missing{
					Item:   s.What,
					Why:    why,
					Steps:  s.Guide,
					Fix:    FixInput,
					Secret: true,
					Accept: func(v string) error { return d2.Put(k, v) },
				})
			}
		}
	}

	return inputMissing, nil
}

// Release deploys the plan via CI.
func (d Deployment) Release() (Run, error) {
	missing, err := d.Check()
	if err != nil {
		return Run{}, err
	}
	if len(missing) > 0 {
		var items []string
		for _, m := range missing {
			items = append(items, string(m.Item))
		}
		return Run{}, fmt.Errorf("%w: %s", ErrNotReady, strings.Join(items, ", "))
	}
	return d.CI.Release(d.Plan)
}

// Status returns the current status of a run.
func (d Deployment) Status(r Run) (RunState, error) {
	if err := validate(d.Plan); err != nil {
		return 0, err
	}
	if err := d.checkTools(); err != nil {
		return 0, err
	}
	return d.CI.Status(r)
}

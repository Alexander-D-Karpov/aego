package world

type Config struct {
	Gravity               Vec2
	Substeps              int
	VelocityIterations    int
	RelaxIterations       int
	LinearSlop            float32
	ContactOffset         float32
	ContactHertz          float32
	ContactDampingRatio   float32
	ContactPushVelocity   float32
	RestitutionThreshold  float32
	MaxTranslationPerStep float32
	MaxRotationPerStep    float32
	SleepLinear           float32
	SleepAngular          float32
	SleepTime             float32
	InitialBodies         int
	InitialShapes         int
	InitialContacts       int
	DebugVectors          bool
}

func DefaultConfig() Config {
	return Config{
		Gravity:               Vec2{Y: 9.81},
		Substeps:              4,
		VelocityIterations:    1,
		RelaxIterations:       1,
		LinearSlop:            0.005,
		ContactOffset:         0.02,
		ContactHertz:          30,
		ContactDampingRatio:   10,
		ContactPushVelocity:   3,
		RestitutionThreshold:  1,
		MaxTranslationPerStep: 2,
		MaxRotationPerStep:    0.5 * 3.14159265,
		SleepLinear:           0.01,
		SleepAngular:          2 * 3.14159265 / 180,
		SleepTime:             0.5,
		InitialBodies:         256,
		InitialShapes:         256,
		InitialContacts:       1024,
	}
}

func (c Config) withDefaults() Config {
	d := DefaultConfig()
	setI := func(v *int, def int) {
		if *v == 0 {
			*v = def
		}
	}
	setF := func(v *float32, def float32) {
		if *v == 0 {
			*v = def
		}
	}
	setI(&c.Substeps, d.Substeps)
	setI(&c.VelocityIterations, d.VelocityIterations)
	setI(&c.RelaxIterations, d.RelaxIterations)
	setF(&c.LinearSlop, d.LinearSlop)
	setF(&c.ContactOffset, d.ContactOffset)
	setF(&c.ContactHertz, d.ContactHertz)
	setF(&c.ContactDampingRatio, d.ContactDampingRatio)
	setF(&c.ContactPushVelocity, d.ContactPushVelocity)
	setF(&c.RestitutionThreshold, d.RestitutionThreshold)
	setF(&c.MaxTranslationPerStep, d.MaxTranslationPerStep)
	setF(&c.MaxRotationPerStep, d.MaxRotationPerStep)
	setF(&c.SleepLinear, d.SleepLinear)
	setF(&c.SleepAngular, d.SleepAngular)
	setF(&c.SleepTime, d.SleepTime)
	if c.InitialBodies <= 0 {
		c.InitialBodies = d.InitialBodies
	}
	if c.InitialShapes <= 0 {
		c.InitialShapes = d.InitialShapes
	}
	if c.InitialContacts <= 0 {
		c.InitialContacts = d.InitialContacts
	}
	return c
}

func (c Config) Validate() error {
	switch {
	case !finite2(c.Gravity):
		return bad("gravity is not finite")
	case c.Substeps < 1 || c.Substeps > 64:
		return bad("substeps %d is outside 1..64", c.Substeps)
	case c.VelocityIterations < 1 || c.VelocityIterations > 64:
		return bad("velocity iterations %d is outside 1..64", c.VelocityIterations)
	case c.RelaxIterations < 1 || c.RelaxIterations > 64:
		return bad("relax iterations %d is outside 1..64", c.RelaxIterations)
	}
	for _, f := range [...]struct {
		name string
		v    float32
	}{
		{"linear slop", c.LinearSlop},
		{"contact offset", c.ContactOffset},
		{"contact hertz", c.ContactHertz},
		{"contact damping ratio", c.ContactDampingRatio},
		{"contact push velocity", c.ContactPushVelocity},
		{"restitution threshold", c.RestitutionThreshold},
		{"max translation", c.MaxTranslationPerStep},
		{"max rotation", c.MaxRotationPerStep},
		{"sleep linear", c.SleepLinear},
		{"sleep angular", c.SleepAngular},
		{"sleep time", c.SleepTime},
	} {
		if !finite(f.v) || f.v <= 0 {
			return bad("%s must be positive and finite, got %v", f.name, f.v)
		}
	}
	if c.ContactOffset < c.LinearSlop {
		return bad("contact offset %v is smaller than linear slop %v", c.ContactOffset, c.LinearSlop)
	}
	return nil
}

func (c Config) WithDefaults() Config {
	return c.withDefaults()
}

package model

// Traffic says how load arrives in the words people use for it: a rate, a
// number of users and what each does, users working at the same time, a
// schedule, or batches. The traffic package turns it into a Load. A node has
// either a Load or a Traffic, never both.
type Traffic struct {
	// Exactly one of these shapes.
	Rate       *Rate       `yaml:"rate,omitempty" json:"rate,omitempty"`
	Users      *Users      `yaml:"users,omitempty" json:"users,omitempty"`
	Concurrent *Concurrent `yaml:"concurrent,omitempty" json:"concurrent,omitempty"`
	// Schedule is rate() or cron() (EventBridge), five-field Unix cron
	// (Cloud Scheduler) or six-field cron with seconds (Azure timers).
	Schedule string `yaml:"schedule,omitempty" json:"schedule,omitempty"`
	Batch    *Batch `yaml:"batch,omitempty" json:"batch,omitempty"`

	// When the traffic comes, for rate, users and concurrent: hours of the
	// day ("9-18", "9-12,13-18"; all day when empty) and days ("all",
	// "weekdays", "weekends").
	Hours string `yaml:"hours,omitempty" json:"hours,omitempty"`
	Days  string `yaml:"days,omitempty" json:"days,omitempty"`
	// PeakFactor multiplies the average rate over the active hours into the
	// peak; PeakPerSecond sets the peak outright.
	PeakFactor    *float64 `yaml:"peakFactor,omitempty" json:"peakFactor,omitempty"`
	PeakPerSecond *float64 `yaml:"peakPerSecond,omitempty" json:"peakPerSecond,omitempty"`
}

// Rate is Count arrivals per Per: second, minute or hour while active, or in
// total per day, week or month.
type Rate struct {
	Count float64 `yaml:"count" json:"count"`
	Per   string  `yaml:"per" json:"per"`
}

// Users is Count people who each do Actions things per Per (day, week or month).
type Users struct {
	Count   float64 `yaml:"count" json:"count"`
	Actions float64 `yaml:"actions" json:"actions"`
	Per     string  `yaml:"per" json:"per"`
}

// Concurrent is a closed model: Users people at work at the same time, each
// doing one thing every EverySeconds (think time plus response time).
type Concurrent struct {
	Users        float64 `yaml:"users" json:"users"`
	EverySeconds float64 `yaml:"everySeconds" json:"everySeconds"`
}

// Batch is Items units that arrive together on a schedule (Every) and are
// worked through within WithinSeconds.
type Batch struct {
	Items         float64 `yaml:"items" json:"items"`
	Every         string  `yaml:"every" json:"every"`
	WithinSeconds float64 `yaml:"withinSeconds" json:"withinSeconds"`
}

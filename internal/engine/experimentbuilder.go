package engine

import (
	"errors"
	"fmt"
	"reflect"
	"regexp"
	"strconv"

	"github.com/iancoleman/strcase"
	"github.com/ooni/probe-cli/v3/internal/model"
)

// InputPolicy describes the experiment policy with respect to input. That is
// whether it requires input, optionally accepts input, does not want input.
type InputPolicy string

const (
	// InputOrQueryBackend indicates that the experiment requires
	// external input to run and that this kind of input is URLs
	// from the citizenlab/test-lists repository. If this input
	// not provided to the experiment, then the code that runs the
	// experiment is supposed to fetch from URLs from OONI's backends.
	InputOrQueryBackend = InputPolicy("or_query_backend")

	// InputStrictlyRequired indicates that the experiment
	// requires input and we currently don't have an API for
	// fetching such input. Therefore, either the user specifies
	// input or the experiment will fail for the lack of input.
	InputStrictlyRequired = InputPolicy("strictly_required")

	// InputOptional indicates that the experiment handles input,
	// if any; otherwise it fetchs input/uses a default.
	InputOptional = InputPolicy("optional")

	// InputNone indicates that the experiment does not want any
	// input and ignores the input if provided with it.
	InputNone = InputPolicy("none")

	// We gather input from StaticInput and SourceFiles. If there is
	// input, we return it. Otherwise, we return an internal static
	// list of inputs to be used with this experiment.
	InputOrStaticDefault = InputPolicy("or_static_default")
)

// ExperimentBuilder is an experiment builder.
type ExperimentBuilder struct {
	build         func(interface{}) *Experiment
	callbacks     model.ExperimentCallbacks
	config        interface{}
	inputPolicy   InputPolicy
	interruptible bool
}

// Interruptible tells you whether this is an interruptible experiment. This kind
// of experiments (e.g. ndt7) may be interrupted mid way.
func (b *ExperimentBuilder) Interruptible() bool {
	return b.interruptible
}

// InputPolicy returns the experiment input policy
func (b *ExperimentBuilder) InputPolicy() InputPolicy {
	return b.inputPolicy
}

// OptionInfo contains info about an option
type OptionInfo struct {
	Doc  string
	Type string
}

// Options returns info about all options
func (b *ExperimentBuilder) Options() (map[string]OptionInfo, error) {
	result := make(map[string]OptionInfo)
	ptrinfo := reflect.ValueOf(b.config)
	if ptrinfo.Kind() != reflect.Ptr {
		return nil, errors.New("config is not a pointer")
	}
	structinfo := ptrinfo.Elem().Type()
	if structinfo.Kind() != reflect.Struct {
		return nil, errors.New("config is not a struct")
	}
	for i := 0; i < structinfo.NumField(); i++ {
		field := structinfo.Field(i)
		result[field.Name] = OptionInfo{
			Doc:  field.Tag.Get("ooni"),
			Type: field.Type.String(),
		}
	}
	return result, nil
}

// SetOptionBool sets a bool option
func (b *ExperimentBuilder) SetOptionBool(key string, value bool) error {
	field, err := fieldbyname(b.config, key)
	if err != nil {
		return err
	}
	if field.Kind() != reflect.Bool {
		return errors.New("field is not a bool")
	}
	field.SetBool(value)
	return nil
}

// SetOptionInt sets an int option
func (b *ExperimentBuilder) SetOptionInt(key string, value int64) error {
	field, err := fieldbyname(b.config, key)
	if err != nil {
		return err
	}
	if field.Kind() != reflect.Int64 {
		return errors.New("field is not an int64")
	}
	field.SetInt(value)
	return nil
}

// SetOptionString sets a string option
func (b *ExperimentBuilder) SetOptionString(key, value string) error {
	field, err := fieldbyname(b.config, key)
	if err != nil {
		return err
	}
	if field.Kind() != reflect.String {
		return errors.New("field is not a string")
	}
	field.SetString(value)
	return nil
}

var intregexp = regexp.MustCompile("^[0-9]+$")

// SetOptionGuessType sets an option whose type depends on the
// option value. If the value is `"true"` or `"false"` we
// assume the option is boolean. If the value is numeric, then we
// set an integer option. Otherwise we set a string option.
func (b *ExperimentBuilder) SetOptionGuessType(key, value string) error {
	if value == "true" || value == "false" {
		return b.SetOptionBool(key, value == "true")
	}
	if !intregexp.MatchString(value) {
		return b.SetOptionString(key, value)
	}
	number, _ := strconv.ParseInt(value, 10, 64)
	return b.SetOptionInt(key, number)
}

// SetOptionsGuessType calls the SetOptionGuessType method for every
// key, value pair contained by the opts input map.
func (b *ExperimentBuilder) SetOptionsGuessType(opts map[string]string) error {
	for k, v := range opts {
		if err := b.SetOptionGuessType(k, v); err != nil {
			return err
		}
	}
	return nil
}

// SetCallbacks sets the interactive callbacks
func (b *ExperimentBuilder) SetCallbacks(callbacks model.ExperimentCallbacks) {
	b.callbacks = callbacks
}

func fieldbyname(v interface{}, key string) (reflect.Value, error) {
	// See https://stackoverflow.com/a/6396678/4354461
	ptrinfo := reflect.ValueOf(v)
	if ptrinfo.Kind() != reflect.Ptr {
		return reflect.Value{}, errors.New("value is not a pointer")
	}
	structinfo := ptrinfo.Elem()
	if structinfo.Kind() != reflect.Struct {
		return reflect.Value{}, errors.New("value is not a pointer to struct")
	}
	field := structinfo.FieldByName(key)
	if !field.IsValid() || !field.CanSet() {
		return reflect.Value{}, errors.New("no such field")
	}
	return field, nil
}

// NewExperiment creates the experiment
func (b *ExperimentBuilder) NewExperiment() *Experiment {
	experiment := b.build(b.config)
	experiment.callbacks = b.callbacks
	return experiment
}

// canonicalizeExperimentName allows code to provide experiment names
// in a more flexible way, where we have aliases.
//
// Because we allow for uppercase experiment names for backwards
// compatibility with MK, we need to add some exceptions here when
// mapping (e.g., DNSCheck => dnscheck).
func canonicalizeExperimentName(name string) string {
	switch name = strcase.ToSnake(name); name {
	case "ndt_7":
		name = "ndt" // since 2020-03-18, we use ndt7 to implement ndt by default
	case "dns_check":
		name = "dnscheck"
	case "stun_reachability":
		name = "stunreachability"
	default:
	}
	return name
}

func newExperimentBuilder(session *Session, name string) (*ExperimentBuilder, error) {
	factory := experimentsByName[canonicalizeExperimentName(name)]
	if factory == nil {
		return nil, fmt.Errorf("no such experiment: %s", name)
	}
	builder := factory(session)
	builder.callbacks = model.NewPrinterCallbacks(session.Logger())
	return builder, nil
}

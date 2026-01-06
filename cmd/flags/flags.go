package flags

import (
	"encoding/csv"
	"os"
	"strconv"
	"strings"

	"github.com/skevetter/devpod/pkg/platform"
	"github.com/skevetter/log"
	flag "github.com/spf13/pflag"
)

type GlobalFlags struct {
	Context    string
	Provider   string
	AgentDir   string
	DevPodHome string
	UID        string
	Owner      platform.OwnerFilter

	LogOutput string
	Debug     bool
	Silent    bool
}

const DevpodEnvPrefix = "DEVPOD_"

// Defines a string flag with specified name, environment variable, default value, and usage string.
// The argument variable points to a string variable in which to store the value of the flag.
func StringVarE(f *flag.FlagSet, variable *string, name string, environmentVariable string, defaultValue string, usage string) {
	f.StringVar(variable, name, GetStringEnv(environmentVariable, defaultValue), usage+". You can also use "+environmentVariable+" to set this")
}

func GetStringEnv(environmentVariable string, defaultValue string) string {
	if value, exists := os.LookupEnv(environmentVariable); exists {
		return value
	}
	return defaultValue
}

// Defines a bool flag with specified name, environment variable, default value, and usage string.
// The argument variable points to a bool variable in which to store the value of the flag.
func BoolVarE(f *flag.FlagSet, variable *bool, name string, environmentVariable string, defaultValue bool, usage string) {
	f.BoolVar(variable, name, GetBoolEnv(environmentVariable, defaultValue), usage+". You can also use "+environmentVariable+" to set this")
}

func GetBoolEnv(environmentVariable string, defaultValue bool) bool {
	if value, exists := os.LookupEnv(environmentVariable); exists {
		result, err := strconv.ParseBool(value)
		if err != nil {
			log.Default.Warnf("invalid boolean value %s for environment variable %s, falling back to default %v", value, environmentVariable, defaultValue)
			return defaultValue
		}
		return result
	}
	return defaultValue
}

// Defines a string flag with specified name, environment variable, default value, and usage string.
// The argument variable points to a []string variable in which to store the values of the multiple flags.
// The value of each argument will not try to be separated by comma. Use a StringSliceVar for that.
func StringArrayVarE(f *flag.FlagSet, variable *[]string, name string, environmentVariable string, defaultValue []string, usage string) {
	f.StringArrayVar(variable, name, GetStringSliceEnv(environmentVariable, defaultValue), usage)
}

// Defines a string flag with specified name, environment variable, default value, and usage string.
// The argument variable points to a []string variable in which to store the value of the flag.
// Compared to StringArrayVar flags, StringSliceVar flags take comma-separated value as arguments and split them accordingly.
func StringSliceVarE(f *flag.FlagSet, variable *[]string, name string, environmentVariable string, defaultValue []string, usage string) {
	f.StringSliceVar(variable, name, GetStringSliceEnv(environmentVariable, defaultValue), usage)
}

func GetStringSliceEnv(environmentVariable string, defaultValue []string) []string {
	if value, exists := os.LookupEnv(environmentVariable); exists {
		if value == "" {
			return []string{}
		}
		stringReader := strings.NewReader(value)
		csvReader := csv.NewReader(stringReader)
		result, err := csvReader.Read()
		if err != nil {
			log.Default.Warnf("invalid string list value %s for environment variable %s, falling back to default %v", value, environmentVariable, defaultValue)
			return defaultValue
		}
		return result
	}
	return defaultValue
}

// SetGlobalFlags applies the global flags
func SetGlobalFlags(flags *flag.FlagSet) *GlobalFlags {
	globalFlags := &GlobalFlags{}

	flags.StringVar(&globalFlags.DevPodHome, "devpod-home", "", "If defined will override the default devpod home. You can also use DEVPOD_HOME to set this")
	StringVarE(flags, &globalFlags.LogOutput, "log-output", DevpodEnvPrefix+"LOG_OUTPUT", "plain", "The log format to use. Can be either plain, raw or json")
	StringVarE(flags, &globalFlags.Context, "context", DevpodEnvPrefix+"CONTEXT", "", "The context to use")
	StringVarE(flags, &globalFlags.Provider, "provider", DevpodEnvPrefix+"PROVIDER", "", "The provider to use. Needs to be configured for the selected context")
	BoolVarE(flags, &globalFlags.Debug, "debug", DevpodEnvPrefix+"DEBUG", false, "Prints the stack trace if an error occurs")
	BoolVarE(flags, &globalFlags.Silent, "silent", DevpodEnvPrefix+"SILENT", false, "Run in silent mode and prevents any devpod log output except panics & fatals")

	flags.Var(&globalFlags.Owner, "owner", "Show pro workspaces for owner")
	_ = flags.MarkHidden("owner")
	flags.StringVar(&globalFlags.UID, "uid", "", "Set UID for workspace")
	_ = flags.MarkHidden("uid")
	flags.StringVar(&globalFlags.AgentDir, "agent-dir", "", "The data folder where agent data is stored.")
	_ = flags.MarkHidden("agent-dir")
	return globalFlags
}

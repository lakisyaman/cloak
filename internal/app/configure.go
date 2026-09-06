package app

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/lakisyaman/cloak/internal/connectors"
	"github.com/lakisyaman/cloak/internal/contextstore"
	"github.com/lakisyaman/cloak/internal/secrets"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

func newContextConfigureCommand(env CommandEnv, name string) *cobra.Command {
	def, loadErr := env.Connectors.Get(name)
	var clear []string
	cmd := &cobra.Command{Use: "configure <name>", Short: "Create or repair Context values (wizard in an interactive terminal)", Args: cobra.ExactArgs(1)}
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		if loadErr != nil {
			return loadErr
		}
		return configureContext(cmd, env, def, args[0], clear)
	}
	cmd.SetFlagErrorFunc(func(cmd *cobra.Command, err error) error {
		return fmt.Errorf("invalid configuration flags; run cloak %s context configure --help", name)
	})
	cmd.Flags().StringSliceVar(&clear, "clear", nil, "clear optional fields by configuration flag name (comma-separated or repeated)")
	if def != nil {
		for _, fieldName := range def.FieldNames() {
			field := def.Fields[fieldName]
			help := field.Type
			if field.Required {
				help += " (required)"
			}
			if field.Secret {
				help += " (stored in keyring)"
			}
			flag := field.FlagName(fieldName)
			cmd.Flags().String(flag, "", help)
			if field.Type == "boolean" {
				cmd.Flags().Lookup(flag).NoOptDefVal = "true"
			}
		}
	}
	return cmd
}

func configureContext(cmd *cobra.Command, env CommandEnv, def *connectors.Definition, name string, clear []string) error {
	if err := validateContextName(name); err != nil {
		return err
	}
	config, err := env.Store.ReadConfig()
	if err != nil {
		return err
	}
	// Work on a detached copy so failed validation/writes cannot alter a Store's
	// in-memory state. New secret references are committed with the metadata.
	encoded, err := json.Marshal(config)
	if err != nil {
		return err
	}
	var detached contextstore.Config
	if err = json.Unmarshal(encoded, &detached); err != nil {
		return err
	}
	config = detached
	old, _ := findContext(config, def.Command, name)
	ctx := contextstore.Context{Metadata: map[string]any{}, Secrets: map[string]secrets.SecretRef{}}
	for key, value := range old.Metadata {
		ctx.Metadata[key] = value
	}
	for key, value := range old.Secrets {
		ctx.Secrets[key] = value
	}
	cleared := map[string]bool{}
	for _, flag := range clear {
		found := ""
		for key, field := range def.Fields {
			if field.FlagName(key) == flag {
				found = key
				break
			}
		}
		if found == "" {
			return fmt.Errorf("unknown field to clear; see configure --help")
		}
		if cmd.Flags().Changed(flag) {
			return fmt.Errorf("cannot set and clear --%s together", flag)
		}
		delete(ctx.Metadata, found)
		delete(ctx.Secrets, found)
		cleared[found] = true
	}
	interactive := isInteractive(cmd)
	if env.Interactive != nil {
		interactive = env.Interactive()
	}
	newSecrets := map[string]string{}
	for _, key := range def.FieldNames() {
		field := def.Fields[key]
		flag := field.FlagName(key)
		changed := cmd.Flags().Changed(flag)
		var raw any = ctx.Metadata[key]
		hasSecret := false
		var storedSecret string
		if field.Secret {
			if ref, ok := ctx.Secrets[key]; ok {
				if value, getErr := env.Secrets.Get(ref); getErr == nil && value != "" {
					hasSecret = true
					storedSecret = value
				} else if !changed && !interactive && !cleared[key] {
					return fmt.Errorf("secret %s is unavailable; pass --%s to repair it", key, flag)
				}
			}
			// Never migrate plaintext into a keyring silently or show it in a prompt.
			raw = nil
		}
		if changed {
			value, _ := cmd.Flags().GetString(flag)
			raw = value
		}
		if interactive && !changed && !cleared[key] {
			label := flag
			if field.Required {
				label += " (required)"
			}
			if field.Secret {
				if hasSecret {
					label += " [stored; enter to keep]"
				}
			} else if raw != nil {
				if _, valueErr := field.Value(raw); valueErr == nil {
					label += fmt.Sprintf(" [%v]", raw)
				} else {
					label += " [invalid; replace]"
				}
			}
			fmt.Fprintf(cmd.ErrOrStderr(), "%s: ", label)
			var value string
			if field.Secret {
				if env.ReadSecret != nil {
					value, err = env.ReadSecret()
				} else {
					file, ok := cmd.InOrStdin().(*os.File)
					if !ok {
						return fmt.Errorf("secret prompt requires a terminal")
					}
					value, err = readTerminalSecret(file)
				}
				fmt.Fprintln(cmd.ErrOrStderr())
			} else {
				value, err = readPromptLine(cmd.InOrStdin())
			}
			if err != nil {
				return fmt.Errorf("Context wizard interrupted; no changes saved")
			}
			if value != "" {
				raw = value
				changed = true
			}
		}
		if field.Secret && !changed && hasSecret {
			if _, err := field.Value(storedSecret); err != nil {
				return fmt.Errorf("invalid stored --%s: %w; supply --%s to repair it", flag, err, flag)
			}
			delete(ctx.Metadata, key)
			continue
		}
		value, valueErr := field.Value(raw)
		if valueErr != nil {
			return fmt.Errorf("invalid --%s: %w", flag, valueErr)
		}
		if value == nil && field.Required {
			return fmt.Errorf("missing required field %s; pass --%s or run cloak %s context configure %s in an interactive terminal", key, flag, def.Command, name)
		}
		delete(ctx.Metadata, key)
		delete(ctx.Secrets, key)
		if value == nil {
			continue
		}
		if field.Secret {
			newSecrets[key] = value.(string)
		} else {
			ctx.Metadata[key] = value
		}
	}
	created := []secrets.SecretRef{}
	rollback := func() {
		for _, ref := range created {
			_ = env.Secrets.Delete(ref)
		}
	}
	for _, key := range def.FieldNames() {
		value, ok := newSecrets[key]
		if !ok {
			continue
		}
		var nonce [16]byte
		if _, err = rand.Read(nonce[:]); err != nil {
			rollback()
			return err
		}
		ref := secrets.NewRef(def.Command, name, key+"/"+hex.EncodeToString(nonce[:]))
		created = append(created, ref)
		if err = env.Secrets.Set(ref, value); err != nil {
			rollback()
			return fmt.Errorf("could not store secret %s", key)
		}
		ctx.Secrets[key] = ref
	}
	if config.ManagedCLIs == nil {
		config.ManagedCLIs = map[string]contextstore.ManagedCLIConfig{}
	}
	managed := config.ManagedCLIs[def.Command]
	if managed.Contexts == nil {
		managed.Contexts = map[string]contextstore.Context{}
	}
	managed.Contexts[name] = ctx
	config.ManagedCLIs[def.Command] = managed
	if err = env.Store.WriteConfig(config); err != nil {
		rollback()
		return err
	}
	deleteOldSecrets(cmd.ErrOrStderr(), env.Secrets, old, ctx)
	fmt.Fprintf(cmd.OutOrStdout(), "configured context %s for %s\n", name, def.Command)
	return nil
}

// Text prompts must not buffer ahead into the next (possibly secret) answer.
func readPromptLine(reader io.Reader) (string, error) {
	var line strings.Builder
	var next [1]byte
	for {
		if _, err := io.ReadFull(reader, next[:]); err != nil {
			return "", err
		}
		if next[0] == '\n' {
			return strings.TrimSuffix(line.String(), "\r"), nil
		}
		line.WriteByte(next[0])
	}
}

func readTerminalSecret(file *os.File) (string, error) {
	state, err := term.MakeRaw(int(file.Fd()))
	if err != nil {
		return "", err
	}
	defer term.Restore(int(file.Fd()), state)
	// In raw mode Ctrl-C is handled by ReadPassword as cancellation, allowing
	// the deferred restoration to run before returning to the shell.
	terminal := term.NewTerminal(struct {
		io.Reader
		io.Writer
	}{promptReader{file}, io.Discard}, "")
	return terminal.ReadPassword("")
}

type promptReader struct{ io.Reader }

func (r promptReader) Read(buf []byte) (int, error) {
	if len(buf) > 1 {
		buf = buf[:1]
	}
	return r.Reader.Read(buf)
}

func isInteractive(cmd *cobra.Command) bool {
	input, ok := cmd.InOrStdin().(*os.File)
	if !ok {
		return false
	}
	output, ok := cmd.ErrOrStderr().(*os.File)
	if !ok {
		return false
	}
	return term.IsTerminal(int(input.Fd())) && term.IsTerminal(int(output.Fd()))
}

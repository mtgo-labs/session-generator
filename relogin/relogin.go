// Package relogin re-authenticates existing Telegram sessions as fresh logins.
//
// Given an existing MTProto session (string or SQLite file), the pipeline:
//  1. Connects using the old session and retrieves the account's phone number.
//  2. Generates a fresh device profile via device-manager.
//  3. Starts a completely new authentication flow (SendCode → SignIn).
//  4. Exports the brand-new session with a fresh auth key.
//
// The old session is never modified; a new auth key is created server-side.
// This is useful for rotating device fingerprints, migrating between API IDs,
// or refreshing sessions that may be flagged.
//
// # Quick start
//
//	result, err := relogin.Relogin(ctx, relogin.Options{
//	    APIID:      12345,
//	    APIHash:    "abc123",
//	    Session:    "1BVtsOH8Bu...",
//	    Format:     tgconv.FormatPyrogram,
//	    CodeFunc:   relogin.TerminalCodeFunc,
//	    PassFunc:   relogin.TerminalPasswordFunc,
//	    DeviceType: device.Android,
//	})
//	if err != nil { log.Fatal(err) }
//	fmt.Println(result.Session)
package relogin

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"

	device "github.com/mtgo-labs/device-manager"
	tgconv "github.com/mtgo-labs/session-converter"
	"github.com/mtgo-labs/mtgo/telegram"
)

// ErrEmptyPhone is returned when the authenticated account has no phone number
// (e.g. a bot session). Relogin only works for user accounts.
var ErrEmptyPhone = errors.New("relogin: account has no phone number (bot sessions are not supported)")

// ErrInvalidSession is returned when the input session string cannot be decoded.
var ErrInvalidSession = errors.New("relogin: invalid or expired session string")

// Options configures a single re-authentication run.
type Options struct {
	// APIID is the Telegram API ID from my.telegram.org (required).
	APIID int64

	// APIHash is the Telegram API Hash from my.telegram.org (required).
	APIHash string

	// Session is the existing session string to re-authenticate.
	// Any supported format is auto-detected (Telethon, Pyrogram, GramJS, etc.).
	// Required.
	Session string

	// Format is the output format for the new session string.
	// Defaults to telethon.
	Format tgconv.Format

	// CodeFunc returns the login code entered by the user.
	// If nil, telegram.TerminalCodeFunc() is used.
	CodeFunc telegram.CodeFunc

	// PassFunc returns the 2FA password.
	// If nil, telegram.TerminalPasswordFunc() is used.
	PassFunc telegram.PasswordFunc

	// DeviceType selects the device profile for the new session.
	// Defaults to device.Android.
	DeviceType device.Device

	// DeviceID controls deterministic device generation. The same ID
	// always yields the same model/version. If empty, a random profile
	// is generated each run.
	DeviceID string

	// Logger receives progress messages. If nil, slog.Default() is used.
	Logger *slog.Logger

	// Output receives progress messages in human-readable form.
	// If nil, messages go to the Logger only.
	Output io.Writer
}

// Result holds the outcome of a successful re-authentication.
type Result struct {
	// PhoneNumber is the phone number used for the new login.
	PhoneNumber string

	// UserID is the Telegram user ID of the account.
	UserID int64

	// FirstName is the account's first name.
	FirstName string

	// LastName is the account's last name.
	LastName string

	// Username is the account's @username (may be empty).
	Username string

	// Session is the exported session string in the requested format.
	Session string

	// Device is the device profile used for the new session.
	Device device.Profile
}

// Relogin re-authenticates an existing session as a fresh login.
//
// The pipeline connects with the old session, retrieves the phone number,
// generates a new device profile, starts a new authentication flow, and
// exports a brand-new session string.
func Relogin(ctx context.Context, opts Options) (*Result, error) {
	if err := validateOptions(&opts); err != nil {
		return nil, err
	}

	log := opts.logger()
	out := opts.output()

	// Phase 1: Load old session.
	log.Debug("relogin: decoding session string")
	src, _, err := tgconv.Decode(opts.Session)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidSession, err)
	}

	// Phase 2: Connect with old session and get phone number.
	log.Debug("relogin: connecting with existing session")
	phone, user, err := getPhoneFromSession(ctx, opts, src)
	if err != nil {
		return nil, err
	}
	fmt.Fprintf(out, "relogin: phone %s, user %d (%s %s)\n", phone, user.ID, user.FirstName, user.LastName)

	// Phase 3: Generate device profile.
	profile := opts.DeviceType.Generate(opts.DeviceID)
	log.Debug("relogin: device profile", "model", profile.DeviceModel, "system", profile.SystemVersion)
	fmt.Fprintf(out, "relogin: device %s (%s)\n", profile.DeviceModel, profile.SystemVersion)

	// Phase 4: Fresh authentication.
	log.Debug("relogin: starting fresh authentication")
	session, err := freshAuth(ctx, opts, phone, profile, out)
	if err != nil {
		return nil, err
	}

	// Phase 5: Convert to target format.
	log.Debug("relogin: converting session", "format", opts.Format)
	output, err := tgconv.Convert(session, opts.Format)
	if err != nil {
		return nil, fmt.Errorf("relogin: convert to %s: %w", opts.Format, err)
	}

	fmt.Fprintf(out, "relogin: success — new session exported\n")
	return &Result{
		PhoneNumber: phone,
		UserID:      user.ID,
		FirstName:   user.FirstName,
		LastName:    user.LastName,
		Username:    user.Username,
		Session:     output,
		Device:      profile,
	}, nil
}

// SessionResult pairs a session input with its relogin outcome.
// Used by ReloginBatch.
type SessionResult struct {
	// Input is the original session string passed to ReloginBatch.
	Input string

	// Result is the successful outcome, or nil if Error is non-nil.
	Result *Result

	// Error is non-nil if this session failed.
	Error error
}

// ReloginBatch re-authenticates multiple sessions sequentially.
// Each session is processed independently — a failure on one does not stop
// the others. Results are returned in the same order as the input.
func ReloginBatch(ctx context.Context, opts Options, sessions []string) []*SessionResult {
	results := make([]*SessionResult, len(sessions))
	for i, sess := range sessions {
		o := opts
		o.Session = sess

		result, err := Relogin(ctx, o)
		results[i] = &SessionResult{
			Input:  sess,
			Result: result,
			Error:  err,
		}

		if err != nil {
			opts.logger().Error("relogin: batch item failed", "index", i, "error", err)
			fmt.Fprintf(opts.output(), "relogin: [%d/%d] FAILED: %v\n", i+1, len(sessions), err)
		} else {
			fmt.Fprintf(opts.output(), "relogin: [%d/%d] OK — user %d\n", i+1, len(sessions), result.UserID)
		}
	}
	return results
}

// --- internal helpers ---

func validateOptions(opts *Options) error {
	if opts.APIID == 0 {
		return errors.New("relogin: APIID is required")
	}
	if opts.APIHash == "" {
		return errors.New("relogin: APIHash is required")
	}
	if opts.Session == "" {
		return errors.New("relogin: Session is required")
	}
	if opts.CodeFunc == nil {
		opts.CodeFunc = telegram.TerminalCodeFunc()
	}
	if opts.PassFunc == nil {
		opts.PassFunc = telegram.TerminalPasswordFunc()
	}
	if opts.DeviceType == "" {
		opts.DeviceType = device.Android
	}
	if opts.Format == "" {
		opts.Format = tgconv.FormatTelethon
	}
	return nil
}

func (o *Options) logger() *slog.Logger {
	if o.Logger != nil {
		return o.Logger
	}
	return slog.Default()
}

func (o *Options) output() io.Writer {
	if o.Output != nil {
		return o.Output
	}
	return io.Discard
}

// getPhoneFromSession connects with the old session and returns the phone
// number and user info. The client is disconnected before returning.
func getPhoneFromSession(ctx context.Context, opts Options, src *tgconv.Session) (string, userBrief, error) {
	// Encode as Pyrogram for import into mtgo (SessionString config field).
	pyroStr, err := tgconv.Encode(src, tgconv.FormatPyrogram)
	if err != nil {
		return "", userBrief{}, fmt.Errorf("relogin: encode for import: %w", err)
	}

	cfg := &telegram.Config{
		APIID:       int32(opts.APIID),
		APIHash:     opts.APIHash,
		SessionString: pyroStr,
		InMemory:    true,
		NoUpdates:   true,
	}

	client, err := telegram.NewClient(int32(opts.APIID), opts.APIHash, cfg)
	if err != nil {
		return "", userBrief{}, fmt.Errorf("relogin: create old-session client: %w", err)
	}

	if err := client.Connect(0); err != nil {
		return "", userBrief{}, fmt.Errorf("relogin: connect with old session: %w", err)
	}
	defer client.Stop()

	me, err := client.GetMe(ctx)
	if err != nil {
		return "", userBrief{}, fmt.Errorf("relogin: GetMe: %w", err)
	}

	phone := strings.TrimSpace(me.Phone)
	if phone == "" {
		return "", userBrief{}, ErrEmptyPhone
	}
	// Telegram returns phone without the leading +, SendCode needs it.
	if !strings.HasPrefix(phone, "+") {
		phone = "+" + phone
	}

	return phone, userBrief{
		ID:        me.ID,
		FirstName: me.FirstName,
		LastName:  me.LastName,
		Username:  me.Username,
	}, nil
}

// userBrief carries just the fields we need from GetMe.
type userBrief struct {
	ID        int64
	FirstName string
	LastName  string
	Username  string
}

// freshAuth performs a complete new authentication and returns the exported
// session string (Pyrogram format, before final format conversion).
func freshAuth(ctx context.Context, opts Options, phone string, profile device.Profile, out io.Writer) (string, error) {
	cfg := &telegram.Config{
		APIID:     int32(opts.APIID),
		APIHash:   opts.APIHash,
		InMemory:  true,
		NoUpdates: true,
	}
	profile.Apply(cfg)

	// Wire interactive callbacks.
	cfg.PhoneNumber = phone
	cfg.CodeFunc = opts.CodeFunc
	cfg.PasswordFunc = opts.PassFunc

	client, err := telegram.NewClient(int32(opts.APIID), opts.APIHash, cfg)
	if err != nil {
		return "", fmt.Errorf("relogin: create fresh client: %w", err)
	}

	if err := client.Connect(0); err != nil {
		return "", fmt.Errorf("relogin: connect: %w", err)
	}
	defer client.Stop()

	// Export the new session.
	pyroStr, err := client.ExportSessionString()
	if err != nil {
		return "", fmt.Errorf("relogin: export session: %w", err)
	}
	if pyroStr == "" {
		return "", errors.New("relogin: authentication produced empty session")
	}

	return pyroStr, nil
}

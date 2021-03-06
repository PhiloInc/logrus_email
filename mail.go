package logrus_mail

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net"
	"net/mail"
	"net/smtp"
	"strconv"
	"time"
	"runtime"

	"github.com/Philoinc/logrus"
)

const (
	format = "20060102 15:04:05"
	MAX_DEPTH = 100
)

// MailHook to sends logs by email without authentication.
type MailHook struct {
	AppName string
	c       *smtp.Client
}

// MailAuthHook to sends logs by email with authentication.
type MailAuthHook struct {
	AppName  string
	Host     string
	Port     int
	From     *mail.Address
	To       *mail.Address
	Username string
	Password string
}

// NewMailHook creates a hook to be added to an instance of logger.
func NewMailHook(appname string, host string, port int, from string, to string) (*MailHook, error) {
	// Connect to the remote SMTP server.
	c, err := smtp.Dial(host + ":" + strconv.Itoa(port))
	if err != nil {
		return nil, err
	}

	// Validate sender and recipient
	sender, err := mail.ParseAddress(from)
	if err != nil {
		return nil, err
	}
	recipient, err := mail.ParseAddress(to)
	if err != nil {
		return nil, err
	}

	// Set the sender and recipient.
	if err := c.Mail(sender.Address); err != nil {
		return nil, err
	}
	if err := c.Rcpt(recipient.Address); err != nil {
		return nil, err
	}

	return &MailHook{
		AppName: appname,
		c:       c,
	}, nil

}

// NewMailAuthHook creates a hook to be added to an instance of logger.
func NewMailAuthHook(appname string, host string, port int, from string, to string, username string, password string) (*MailAuthHook, error) {
	// Check if server listens on that port.
	conn, err := net.DialTimeout("tcp", host+":"+strconv.Itoa(port), 3*time.Second)
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	// Validate sender and recipient
	sender, err := mail.ParseAddress(from)
	if err != nil {
		return nil, err
	}
	receiver, err := mail.ParseAddress(to)
	if err != nil {
		return nil, err
	}

	return &MailAuthHook{
		AppName:  appname,
		Host:     host,
		Port:     port,
		From:     sender,
		To:       receiver,
		Username: username,
		Password: password}, nil
}

// Fire is called when a log event is fired.
func (hook *MailHook) Fire(entry *logrus.Entry) error {
	wc, err := hook.c.Data()
	if err != nil {
		return err
	}
	defer wc.Close()
	message := createMessage(entry, hook.AppName, "", "")
	if _, err = message.WriteTo(wc); err != nil {
		return err
	}
	return nil
}

// Fire is called when a log event is fired.
func (hook *MailAuthHook) Fire(entry *logrus.Entry) error {
	message := createMessage(entry, hook.AppName, hook.From.Address, hook.To.Address)

	// Spawn the actual email sending since it appears to interfere with
	// the HTTP request handling when a panic is caught and handled
	// NOTE: It is critical that the message, which includes the stack
	//       trace details, is created before the go routine is called
	go func() {
		// Connect to the server, authenticate, set the sender and recipient,
		// and send the email all in one step.
		auth := smtp.PlainAuth("", hook.Username, hook.Password, hook.Host)
		smtp.SendMail(
			hook.Host+":"+strconv.Itoa(hook.Port),
			auth,
			hook.From.Address,
			[]string{hook.To.Address},
			message.Bytes(),
		)
	}()

	return nil
}

// Levels returns the available logging levels.
func (hook *MailAuthHook) Levels() []logrus.Level {
	return []logrus.Level{
		logrus.PanicLevel,
		logrus.FatalLevel,
		logrus.ErrorLevel,
	}
}

// Levels returns the available logging levels.
func (hook *MailHook) Levels() []logrus.Level {
	return []logrus.Level{
		logrus.PanicLevel,
		logrus.FatalLevel,
		logrus.ErrorLevel,
	}
}

func createMessage(entry *logrus.Entry, appname string, from string, to string) *bytes.Buffer {
	// Cobble together a stack trace as best we can
	trace   := ""
	callers := make([]uintptr, MAX_DEPTH+1)
	depth   := runtime.Callers(1, callers)
	for i := 0; i < depth; i++ {
		pc := callers[i]
		function := runtime.FuncForPC(pc)
		if function == nil {
			break
		}
		name := function.Name()
		entry := function.Entry()
		file, line := function.FileLine(pc)
		trace += fmt.Sprintf("Frame %02d:\r\n", i)
		trace += fmt.Sprintf("\tFile: %s\r\n", file)
		trace += fmt.Sprintf("\tFunction: %s\r\n", name)
		trace += fmt.Sprintf("\tLine: %d\r\n", line)
		trace += fmt.Sprintf("\tPC/Entry: 0x%08x/0x%08x\r\n", pc, entry)
	}
	subject := appname + " - " + entry.Level.String()
	fields, _ := json.MarshalIndent(entry.Data, "", "\t")
	body := entry.Time.Format(format) + " - " + entry.Message + "\r\n\r\n"
	body += trace + "\r\n\r\nData:\r\n\r\n" + fmt.Sprintf("%s", fields)
	contents:= ""
	if from != "" {
		contents += fmt.Sprintf("From: %s\r\n", from)
	}
	if to != "" {
		contents += fmt.Sprintf("To: %s\r\n", to)
	}
	contents += fmt.Sprintf("Subject: %s\r\n\r\n%s\r\n\r\n", subject, body)
	message := bytes.NewBufferString(contents)
	return message
}

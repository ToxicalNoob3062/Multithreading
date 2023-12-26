package main

import (
	"bytes"
	"fmt"
	"html/template"
	"sync"
	"time"

	"github.com/vanng822/go-premailer/premailer"
	mail "github.com/xhit/go-simple-mail/v2"
)

type Mail struct {
	Domain      string
	Host        string
	Port        int
	Username    string
	Password    string
	Encryption  string
	FromAddress string
	FromName    string
	Wait        *sync.WaitGroup
	Mailerchan  chan Message
	Errorchan   chan error
	DoneChan    chan bool
}

type Message struct {
	From        string
	FromName    string
	To          string
	Subject     string
	Attachments []string
	Data        any
	DataMap     map[string]interface{}
	Template    string
}

// a function to listen for messages in the Mailer channel
func (app *Config) listenForMail() {
	for {
		select {
		case msg := <-app.Mailer.Mailerchan:
			go app.Mailer.sendMail(msg, app.Mailer.Errorchan)
		case err := <-app.Mailer.Errorchan:
			app.ErrorLog.Println(err)
		case <-app.Mailer.DoneChan:
		}
	}
}

func (m *Mail) sendMail(msg Message, errorChan chan error) {
	defer m.Wait.Done()

	if msg.Template == "" {
		//send email without template
		msg.Template = "mail"
	}

	if msg.From == "" {
		msg.From = m.FromAddress
	}

	if msg.FromName == "" {
		msg.FromName = m.FromName
	}

	data := map[string]any{
		"message": msg.Data,
	}

	msg.DataMap = data

	//build html mail
	formattedMesage, err := m.buildHTMLMessage(msg)
	if err != nil {
		errorChan <- err
		return
	}

	//build text mail
	plainMesage, err := m.buildTextMessage(msg)
	if err != nil {
		errorChan <- err
		return
	}

	server := mail.NewSMTPClient()
	server.Host = m.Host
	server.Port = m.Port
	server.Username = m.Username
	server.Password = m.Password
	server.Encryption = m.getEncryption(m.Encryption)
	server.KeepAlive = false
	server.ConnectTimeout = 10 * time.Second
	server.SendTimeout = 10 * time.Second

	smtpClient, err := server.Connect()
	if err != nil {
		errorChan <- err
		return
	}
	defer smtpClient.Close()

	email := mail.NewMSG()
	email.SetFrom(msg.From).AddTo(msg.To).SetSubject(msg.Subject)
	email.SetBody(mail.TextHTML, formattedMesage)
	email.AddAlternative(mail.TextPlain, plainMesage)

	for _, attachment := range msg.Attachments {
		email.AddAttachment(attachment)
	}

	err = email.Send(smtpClient)
	if err != nil {
		errorChan <- err
		return
	}
}

func (m *Mail) buildHTMLMessage(msg Message) (string, error) {
	//build template path
	templateToRender := fmt.Sprintf("./cmd/web/templates/%s.html.gohtml", msg.Template)

	//parse template
	t, err := template.New("email-html").ParseFiles(templateToRender)
	if err != nil {
		return "", err
	}

	//create buffer to hold template
	var tpl bytes.Buffer
	err = t.ExecuteTemplate(&tpl, "body", msg.DataMap)
	if err != nil {
		return "", err
	}

	//convert buffer to string
	formattedMessage := tpl.String()
	formattedMessage, err = m.inlineCSS(formattedMessage)
	if err != nil {
		return "", err
	}

	return formattedMessage, nil
}

func (m *Mail) inlineCSS(s string) (string, error) {
	options := premailer.Options{
		RemoveClasses:     true,
		CssToAttributes:   true,
		KeepBangImportant: true,
	}

	// Create a new premailer instance
	prem, err := premailer.NewPremailerFromString(s, &options)
	if err != nil {
		return "", err
	}

	// Transform the HTML using the CSS rules
	html, err := prem.Transform()
	if err != nil {
		return "", err
	}

	return html, nil
}

func (m *Mail) buildTextMessage(msg Message) (string, error) {
	//build template path
	templateToRender := fmt.Sprintf("./cmd/web/templates/%s.plain.gohtml", msg.Template)

	//parse template
	t, err := template.New("email-plain").ParseFiles(templateToRender)
	if err != nil {
		return "", err
	}

	//create buffer to hold template
	var tpl bytes.Buffer
	err = t.ExecuteTemplate(&tpl, "body", msg.DataMap)
	if err != nil {
		return "", err
	}

	//convert buffer to string
	plainMessage := tpl.String()

	return plainMessage, nil
}

func (m *Mail) getEncryption(e string) mail.Encryption {
	switch e {
	case "SSL":
		return mail.EncryptionSSLTLS
	case "TLS":
		return mail.EncryptionSTARTTLS
	case "none":
		return mail.EncryptionNone
	default:
		return mail.EncryptionSTARTTLS
	}
}

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
	From           string
	FromName       string
	To             string
	Subject        string
	Attachments    []string
	AttachmentsMap map[string]string
	Data           any
	DataMap        map[string]interface{}
	Template       string
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
			return
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

	if msg.AttachmentsMap == nil {
		msg.AttachmentsMap = make(map[string]string)
	}

	// data := map[string]any{
	// 	"message": msg.Data,
	// }

	if len(msg.DataMap) == 0 {
		msg.DataMap = make(map[string]interface{})
	}

	msg.DataMap["message"] = msg.Data

	// msg.DataMap = data

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
	defer smtpClient.Close() //for this maybe we are encountring a problem

	email := mail.NewMSG()
	email.SetFrom(msg.From).AddTo(msg.To).SetSubject(msg.Subject)
	email.SetBody(mail.TextHTML, formattedMesage)
	email.AddAlternative(mail.TextPlain, plainMesage)

	if len(msg.Attachments) > 0 {
		for _, x := range msg.Attachments {
			email.AddAttachment(x)
		}
	}

	if len(msg.AttachmentsMap) > 0 {
		for name, path := range msg.AttachmentsMap {
			email.AddAttachment(path, name)
		}
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
		RemoveClasses:     false,
		CssToAttributes:   false,
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
	case "ssl":
		return mail.EncryptionSSLTLS
	case "tls":
		return mail.EncryptionSTARTTLS
	case "none":
		return mail.EncryptionNone
	default:
		return mail.EncryptionSTARTTLS
	}
}

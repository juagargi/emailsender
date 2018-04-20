package main

import (
	"bufio"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"strings"

	"github.com/keighl/postmark"
	"github.com/sec51/goconf"
)

type Config struct {
	pm_server_token  string
	pm_account_token string
	from             string
}

func loadConf() Config {
	return Config{
		pm_server_token:  goconf.AppConf.String("email.pm_server_token"),
		pm_account_token: goconf.AppConf.String("email.pm_account_token"),
		from:             goconf.AppConf.String("email.from"),
	}
}

type Email struct {
	Subject string
	Body    string
	Tag     string
	From    string
	To      []string
}

func loadEmail() Email {
	bytes, err := ioutil.ReadFile("email.txt")
	if err != nil {
		fmt.Println("Error:", err)
		os.Exit(1)
	}
	contents := string(bytes)
	// first line is subject
	// second line is empty
	subject := ""
	i := 0
	for ; i < len(contents); i++ {
		if contents[i] == '\n' {
			// [0,i-1] is the subject
			subject = contents[:i]
			i++
			if i+1 >= len(contents) || contents[i] != '\n' {
				i = len(contents)
			}
			break
		}
	}
	i++
	// body is contents[i:]
	if i >= len(contents) {
		fmt.Println("email.txt must contain the subject in the first line, then empty line, then body")
		os.Exit(1)
	}
	body := contents[i:]

	// now load recipients
	file, err := os.Open("recipients.txt")
	if err != nil {
		fmt.Println("Error:", err)
		os.Exit(1)
	}

	recipients := []string{}
	scanner := bufio.NewScanner(file)
	for lineNumber := 1; scanner.Scan(); lineNumber++ {
		if len(scanner.Text()) > 0 && scanner.Text()[0] == '#' {
			continue
		}
		recipients = append(recipients, scanner.Text())
	}

	return Email{
		Subject: subject,
		Body:    body,
		Tag:     "scionlab-announcement",
		From:    "no-reply@scionlab.org",
		To:      recipients,
	}
}

func askForConfirmation(thisMeansYes string) bool {
	reader := bufio.NewReader(os.Stdin)
	text, _ := reader.ReadString('\n')
	text = text[:len(text)-1]
	return text == thisMeansYes
}

// Send connects to the PostMark email API and sends the email
func Send(conf *Config, mail *Email) error {
	// ask for confirmation
	fmt.Println("---------------------------")
	fmt.Printf("From: %s\nSubject: %s\nBody:\n%s\n", mail.From, mail.Subject, mail.Body)
	fmt.Println("---------------------------")
	fmt.Print("Continue? (y/n) ")
	if !askForConfirmation("y") {
		return errors.New("Cancelled by user")
	}
	allRecipients := strings.Join(mail.To, ",")
	fmt.Println("---------------------------")
	fmt.Println("To:\n", allRecipients)
	fmt.Println("---------------------------")
	fmt.Print("Continue? (y/n) ")
	if !askForConfirmation("y") {
		return errors.New("Cancelled by user")
	}

	fmt.Println("Sending email ...")
	client := postmark.NewClient(conf.pm_server_token, conf.pm_account_token)
	email := postmark.Email{
		From:       mail.From,
		To:         mail.From,
		Bcc:        allRecipients,
		Subject:    mail.Subject,
		TextBody:   mail.Body,
		Tag:        mail.Tag,
		TrackOpens: true,
	}
	_, err := client.SendEmail(email)
	if err != nil {
		fmt.Println("Failed to send email")
		return err
	}
	fmt.Println("Email sent")
	return nil
}

func main() {
	conf := loadConf()
	email := loadEmail()
	err := Send(&conf, &email)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	fmt.Println("Done.")
}

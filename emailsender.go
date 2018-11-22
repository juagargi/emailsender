package main

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"strings"
	"time"

	"github.com/keighl/postmark"
	"github.com/sec51/goconf"
	"golang.org/x/crypto/ssh/terminal"
)

const chunkSize = 49
const fromAddress = "no-reply@scionlab.org"

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
		address := scanner.Text()
		address = strings.Trim(address, " \"'<>")
		if address != "" {
			recipients = append(recipients, address)
		}
	}

	return Email{
		Subject: subject,
		Body:    body,
		Tag:     "scionlab-announcement",
		From:    fromAddress,
		To:      recipients,
	}
}

func askForConfirmation(thisMeansYes string, interactive bool) bool {
	if interactive {
		reader := bufio.NewReader(os.Stdin)
		text, _ := reader.ReadString('\n')
		text = text[:len(text)-1]
		return text == thisMeansYes
	} else {
		// if there is output terminal, use it:
		// fmt.Println("is terminal?:", terminal.IsTerminal(int(os.Stdout.Fd())), "answered")
		if terminal.IsTerminal(int(os.Stdout.Fd())) {
			for i := 10; i > 0; i-- {
				if i%2 == 0 {
					fmt.Printf("\nPress Ctrl-C to Cancel (%d seconds remaining) ...", i)
				}
				time.Sleep(1 * time.Second)
			}
			fmt.Println("")
		} else {
			fmt.Println("y (auto answered, output is not terminal)")
		}
		return true
	}
}

// Send connects to the PostMark email API and sends the email
func Send(conf *Config, mail *Email, interactive bool) error {
	fmt.Println("---------------------------")
	fmt.Printf("From: %s\nSubject: %s\nBody:\n%s\n", mail.From, mail.Subject, mail.Body)
	fmt.Println("---------------------------")
	if interactive {
		// ask for confirmation
		fmt.Print("Continue? (y/n) ")
		if !askForConfirmation("y", interactive) {
			return errors.New("Cancelled by user")
		}
	}
	recipientBuckets := make([][]string, (len(mail.To)-1)/chunkSize+1)
	row := 0
	for ; row < len(recipientBuckets)-1; row++ {
		end := (row + 1) * chunkSize
		recipientBuckets[row] = mail.To[row*chunkSize : end]
	}
	end := len(mail.To)
	recipientBuckets[row] = mail.To[row*chunkSize : end]

	allRecipients := []string{}
	for _, bucket := range recipientBuckets {
		allRecipients = append(allRecipients, strings.Join(bucket, ","))
	}
	fmt.Println("---------------------------")
	fmt.Printf("To: %s\n", strings.Join(allRecipients, "\n"))
	fmt.Println("---------------------------")
	fmt.Print("Continue? (y/n) ")
	if !askForConfirmation("y", interactive) {
		return errors.New("Cancelled by user")
	}
	for bucketIndex, bucket := range recipientBuckets {
		fmt.Printf("Sending chunk %d / %d ... %s\n", bucketIndex+1, len(recipientBuckets), strings.Join(bucket, ","))
		client := postmark.NewClient(conf.pm_server_token, conf.pm_account_token)
		email := postmark.Email{
			From:       mail.From,
			To:         mail.From,
			Bcc:        strings.Join(bucket, ","),
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
	}
	fmt.Println("Email sent")
	return nil
}

func main() {
	var email Email
	toFlag := flag.String("to", "", "Recipients email addresses separated with ;")
	subjectFlag := flag.String("subject", "", "Subject")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage of %s (if one email parameter specified, email body will be read from Stdin):\n", os.Args[0])
		flag.PrintDefaults()
	}
	flag.Parse()
	conf := loadConf()
	interactive := false
	if *toFlag != "" || *subjectFlag != "" {
		reader := bufio.NewReader(os.Stdin)
		b, err := ioutil.ReadAll(reader)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error reading Stdin: %v\n", err)
			os.Exit(1)
		}
		email.Body = string(b)
		if *toFlag == "" || *subjectFlag == "" {
			fmt.Fprintln(os.Stderr, "If setting one field, all fields must be set!")
			flag.Usage()
			os.Exit(1)
		}
		email.From = fromAddress
		email.To = strings.Split(*toFlag, ";")
		email.Subject = *subjectFlag
	} else {
		email = loadEmail()
		interactive = true
	}
	err := Send(&conf, &email, interactive)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	fmt.Println("Done.")
}

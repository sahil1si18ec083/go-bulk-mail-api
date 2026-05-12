package main

import (
	"bytes"
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"io"
	"log"
	"net/smtp"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
)

type SMTPConfig struct {
	Email    string
	Password string
	Host     string
	Port     string
}

var smtpConfig SMTPConfig

func main() {

	err := godotenv.Load()
	if err != nil {
		log.Fatal("Error loading .env")
	}

	smtpConfig = SMTPConfig{
		Email:    os.Getenv("SMTP_EMAIL"),
		Password: os.Getenv("SMTP_PASSWORD"),
		Host:     os.Getenv("SMTP_HOST"),
		Port:     os.Getenv("SMTP_PORT"),
	}

	r := gin.Default()

	r.POST("/send-mail", SendMailHandler)

	port := os.Getenv("PORT")

	if port == "" {
		port = "8080"
	}

	r.Run(":" + port)
}

func SendMailHandler(c *gin.Context) {

	subject := c.PostForm("subject")
	body := c.PostForm("body")

	emails := c.PostFormArray("emails")

	// Remove empty emails
	validEmails := []string{}

	for _, email := range emails {
		email = strings.TrimSpace(email)

		if email != "" {
			validEmails = append(validEmails, email)
		}
	}

	// Max 15 emails
	if len(validEmails) > 15 {
		c.JSON(400, gin.H{
			"error": "maximum 15 emails allowed",
		})
		return
	}

	// File Upload
	file, err := c.FormFile("file")

	var fileData []byte
	var fileName string
	fmt.Print("sports tak ")
	if err == nil {
		fmt.Println(err)
		fmt.Print("sports tak ")

		openedFile, err := file.Open()
		if err != nil {
			c.JSON(500, gin.H{
				"error": "unable to open file",
			})
			return
		}
		defer openedFile.Close()

		fileData, err = io.ReadAll(openedFile)
		if err != nil {
			c.JSON(500, gin.H{
				"error": "unable to read file",
			})
			return
		}

		fileName = file.Filename
		fmt.Println("FILE NAME:", fileName)
		fmt.Println("FILE SIZE:", len(fileData))
	}

	var wg sync.WaitGroup

	success := []string{}
	failed := []string{}

	var mu sync.Mutex

	for _, email := range validEmails {

		wg.Add(1)

		go func(receiver string) {
			defer wg.Done()

			err := SendMail(
				receiver,
				subject,
				body,
				fileName,
				fileData,
			)

			mu.Lock()
			defer mu.Unlock()

			if err != nil {
				fmt.Println(err)
				failed = append(failed, receiver)
			} else {
				success = append(success, receiver)
			}

		}(email)
	}

	wg.Wait()

	c.JSON(200, gin.H{
		"success": success,
		"failed":  failed,
	})
}

func SendMail(
	to string,
	subject string,
	body string,
	fileName string,
	fileData []byte,
) error {

	auth := smtp.PlainAuth(
		"",
		smtpConfig.Email,
		smtpConfig.Password,
		smtpConfig.Host,
	)

	boundary := "my-boundary-779"

	var message bytes.Buffer

	// Headers
	message.WriteString(fmt.Sprintf("From: %s\r\n", smtpConfig.Email))
	message.WriteString(fmt.Sprintf("To: %s\r\n", to))
	message.WriteString(fmt.Sprintf("Subject: %s\r\n", subject))
	message.WriteString("MIME-Version: 1.0\r\n")
	message.WriteString(
		fmt.Sprintf(
			"Content-Type: multipart/mixed; boundary=%s\r\n",
			boundary,
		),
	)

	message.WriteString("\r\n")

	// HTML BODY
	message.WriteString("--" + boundary + "\r\n")
	message.WriteString("Content-Type: text/html; charset=\"UTF-8\"\r\n")
	message.WriteString("Content-Transfer-Encoding: 7bit\r\n")
	message.WriteString("\r\n")

	formattedBody := strings.ReplaceAll(body, "\n", "<br>")

	htmlBody := `
<html>
<body style="
font-family: Arial, sans-serif;
font-size: 14px;
line-height: 1.5;
color: #222222;
">
` + formattedBody + `
</body>
</html>
`

	message.WriteString(htmlBody + "\r\n")

	// PDF ATTACHMENT
	if len(fileData) > 0 {
		fmt.Println("ATTACHMENT SIZE:", len(fileData))
		fmt.Println("ATTACHMENT NAME:", fileName)

		message.WriteString("--" + boundary + "\r\n")

		message.WriteString(
			fmt.Sprintf(
				"Content-Type: application/pdf; name=\"%s\"\r\n",
				filepath.Base(fileName),
			),
		)

		message.WriteString("Content-Transfer-Encoding: base64\r\n")

		message.WriteString(
			fmt.Sprintf(
				"Content-Disposition: attachment; filename=\"%s\"\r\n",
				filepath.Base(fileName),
			),
		)

		message.WriteString("\r\n")

		encoded := make([]byte, base64.StdEncoding.EncodedLen(len(fileData)))
		base64.StdEncoding.Encode(encoded, fileData)

		// 76-char lines
		for i := 0; i < len(encoded); i += 76 {

			end := i + 76

			if end > len(encoded) {
				end = len(encoded)
			}

			message.Write(encoded[i:end])
			message.WriteString("\r\n")
		}
	}

	message.WriteString("--" + boundary + "--")

	addr := smtpConfig.Host + ":" + smtpConfig.Port

	client, err := smtp.Dial(addr)
	if err != nil {
		return err
	}

	defer client.Close()

	tlsConfig := &tls.Config{
		ServerName: smtpConfig.Host,
	}

	err = client.StartTLS(tlsConfig)
	if err != nil {
		return err
	}

	err = client.Auth(auth)
	if err != nil {
		return err
	}

	err = client.Mail(smtpConfig.Email)
	if err != nil {
		return err
	}

	err = client.Rcpt(to)
	if err != nil {
		return err
	}

	w, err := client.Data()
	if err != nil {
		return err
	}

	_, err = w.Write(message.Bytes())
	fmt.Println("FINAL EMAIL SIZE:", len(message.Bytes()))
	if err != nil {
		return err
	}

	err = w.Close()
	if err != nil {
		return err
	}

	return client.Quit()
}

package main

import (
	"fmt"
	"gosub/data"
	"html/template"
	"net/http"
	"strconv"
	"time"

	"github.com/phpdave11/gofpdf"
	"github.com/phpdave11/gofpdf/contrib/gofpdi"
)

func (app *Config) HomePage(w http.ResponseWriter, r *http.Request) {
	app.render(w, r, "home.page.gohtml", nil)
}

func (app *Config) LoginPage(w http.ResponseWriter, r *http.Request) {
	app.render(w, r, "login.page.gohtml", nil)
}

func (app *Config) PostLoginPage(w http.ResponseWriter, r *http.Request) {
	app.Session.RenewToken(r.Context())

	//parse form data
	err := r.ParseForm()
	if err != nil {
		app.ErrorLog.Println(err)
	}

	//get form data
	email := r.Form.Get("email")
	password := r.Form.Get("password")

	//check if user exists
	user, err := app.Models.User.GetByEmail(email)
	if err != nil {
		app.Session.Put(r.Context(), "error", "Email Don't exist")
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	//check if password is correct
	match, err := user.PasswordMatches(password)
	if err != nil {
		app.Session.Put(r.Context(), "error", "Invalid credentials")
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	if !match {
		msg := Message{
			To:      email,
			Subject: "Failed login attempt!!",
			Data:    "InvalidLoginAttempt!!",
		}
		app.sendEmail(msg)
		app.Session.Put(r.Context(), "error", "Wrong password!!")
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	//check if user is active
	if user.Active == 0 {
		app.Session.Put(r.Context(), "error", "Account not activated!!")
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	app.Session.Put(r.Context(), "userID", user.ID)
	app.Session.Put(r.Context(), "user", user)
	app.Session.Put(r.Context(), "flash", "Login successful")

	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (app *Config) Logout(w http.ResponseWriter, r *http.Request) {
	//clean up session
	app.Session.Destroy(r.Context())
	app.Session.RenewToken(r.Context())

	//redirect to login page
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

func (app *Config) RegisterPage(w http.ResponseWriter, r *http.Request) {
	app.render(w, r, "register.page.gohtml", nil)
}

func (app *Config) PostRegisterPage(w http.ResponseWriter, r *http.Request) {
	err := r.ParseForm()
	if err != nil {
		app.ErrorLog.Println(err)
	}
	//create a user
	user := data.User{
		FirstName: r.Form.Get("first-name"),
		LastName:  r.Form.Get("last-name"),
		Email:     r.Form.Get("email"),
		Password:  r.Form.Get("password"),
		Active:    0,
		IsAdmin:   0,
	}

	_, err = user.Insert(user)
	if err != nil {
		app.Session.Put(r.Context(), "error", "Failed to create user")
		http.Redirect(w, r, "/register", http.StatusSeeOther)
		return
	}

	//send activation email
	url := fmt.Sprintf("http://localhost:8000/activate?email=%s", user.Email)
	signedUrl := GenerateTokenFromString(url)

	//make email
	msg := Message{
		To:       user.Email,
		Subject:  "Activate your account!!",
		Template: "confirmation-email",
		Data:     template.HTML(signedUrl),
	}

	//send email
	app.sendEmail(msg)

	//update session
	app.Session.Put(r.Context(), "flash", "Account created successfully. Please check your email to activate your account")

	//redirect to login page
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

func (app *Config) ActivateAccount(w http.ResponseWriter, r *http.Request) {
	//valid url token
	uri := r.RequestURI
	testUrl := fmt.Sprintf("http://localhost:8000%s", uri)
	okay := VerifyToken(testUrl)
	if !okay {
		app.Session.Put(r.Context(), "error", "Invalid token")
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	//get email from url
	user, err := app.Models.User.GetByEmail(r.URL.Query().Get("email"))
	if err != nil {
		app.Session.Put(r.Context(), "error", "No user found!!!")
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	//update user
	user.Active = 1
	err = user.Update()
	if err != nil {
		app.Session.Put(r.Context(), "error", "Unable to update user!!")
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	app.Session.Put(r.Context(), "flash", "Account activated successfully!!")
	http.Redirect(w, r, "/login", http.StatusSeeOther)

}

func (app *Config) SubscribeToPlan(w http.ResponseWriter, r *http.Request) {
	//get the id of the plan
	id := r.URL.Query().Get("id")
	planID, _ := strconv.Atoi(id)

	//get the plan from datbase
	plan, err := app.Models.Plan.GetOne(planID)
	if err != nil {
		app.Session.Put(r.Context(), "error", "Unable to find plan!!")
		http.Redirect(w, r, "/members/plans", http.StatusSeeOther)
		return
	}

	//get the user from session
	user, ok := app.Session.Get(r.Context(), "user").(data.User)
	if !ok {
		app.Session.Put(r.Context(), "error", "Log In first!!!")
		http.Redirect(w, r, "/members/plans", http.StatusSeeOther)
		return
	}

	//generate an invoice and send it via mail
	app.Wait.Add(1)
	go (func() {
		defer app.Wait.Done()

		invoice, err := app.getInvoice(user, plan)
		if err != nil {
			//send this to a channel
			app.ErrorChan <- err
		}

		msg := Message{
			To:       user.Email,
			Subject:  "Your Invoice",
			Data:     invoice,
			Template: "invoice",
		}

		app.sendEmail(msg)
	})()

	//generate a manual
	app.Wait.Add(1)
	go (func() {
		defer app.Wait.Done()

		manual := app.generateManual(user, plan)
		err := manual.OutputFileAndClose(fmt.Sprintf("./tmp/%d_manual.pdf", user.ID))
		if err != nil {
			//send this to a channel
			app.ErrorChan <- err
			return
		}

		msg := Message{
			To:      user.Email,
			Subject: "Your Manual",
			Data:    "Your manual is attached",
			AttachmentsMap: map[string]string{
				"manual.pdf": fmt.Sprintf("./tmp/%d_manual.pdf", user.ID),
			},
		}

		app.sendEmail(msg)

		//test app error chan
		//app.ErrorChan <- fmt.Errorf("test error")
	})()

	//subscribe user to plan
	err = app.Models.Plan.SubscribeUserToPlan(user, *plan)
	if err != nil {
		app.Session.Put(r.Context(), "error", "Error subscribing to plan!!")
		http.Redirect(w, r, "/members/plans", http.StatusSeeOther)
		return
	}

	u, err := app.Models.User.GetOne(user.ID)
	if err != nil {
		app.Session.Put(r.Context(), "error", "Error getting user from database!!")
		http.Redirect(w, r, "/members/plans", http.StatusSeeOther)
		return
	}

	app.Session.Put(r.Context(), "user", u)

	//redirect
	app.Session.Put(r.Context(), "flash", "You have subscribed to a plan!!")
	http.Redirect(w, r, "/members/plans", http.StatusSeeOther)
}

func (app *Config) ChooseSubscription(w http.ResponseWriter, r *http.Request) {
	plans, err := app.Models.Plan.GetAll()
	if err != nil {
		app.ErrorLog.Println(err)
		return
	}

	dataMap := make(map[string]any)
	dataMap["plans"] = plans

	app.render(w, r, "plans.page.gohtml", &TemplateData{
		Data: dataMap,
	})
}

///////////////////////////////UTILITIES///////////////////////////////////////

func (app *Config) generateManual(user data.User, plan *data.Plan) *gofpdf.Fpdf {
	pdf := gofpdf.New("P", "mm", "Letter", "")
	pdf.SetMargins(10, 13, 10)
	importer := gofpdi.NewImporter()
	//let take some time to make sure the file is imported
	time.Sleep(5 * time.Second)

	tmpl := importer.ImportPage(pdf, "./pdf/manual.pdf", 1, "/MediaBox")
	pdf.AddPage()

	importer.UseImportedTemplate(pdf, tmpl, 0, 0, 215.9, 0)

	pdf.SetX(75)
	pdf.SetY(150)
	pdf.SetFont("Arial", "B", 12)
	pdf.MultiCell(0, 4, fmt.Sprintf("%s %s", user.FirstName, user.LastName), "", "C", false)
	pdf.Ln(5)
	pdf.MultiCell(0, 4, fmt.Sprintf("%s User Guide", plan.PlanName), "", "C", false)

	return pdf
}

func (app *Config) getInvoice(user data.User, plan *data.Plan) (string, error) {
	return plan.PlanAmountFormatted, nil
}

package main

import "net/http"

func (app *Config) HomePage(w http.ResponseWriter, r *http.Request) {
	app.render(w, r, "home.page.gohtml", nil)
}

func (app *Config) LoginPage(w http.ResponseWriter, r *http.Request) {
	app.render(w, r, "login.page.gohtml", nil)
}

func (app *Config) PostLoginPage(w http.ResponseWriter, r *http.Request) {
	_ = app.Session.RenewToken(r.Context())

	//pase form data
	err := r.ParseForm()
	if err != nil {
		app.ErrorLog.Println(err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
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
	//app.render(w, r, "login.page.gohtml", nil)
}

func (app *Config) ActivateAccount(w http.ResponseWriter, r *http.Request) {
	//app.render(w, r, "login.page.gohtml", nil)
}

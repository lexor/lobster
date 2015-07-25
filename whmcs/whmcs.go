package whmcs

import "github.com/LunaNode/lobster"
import "github.com/LunaNode/lobster/utils"

import "fmt"
import "log"
import "net/http"
import "strconv"

const TOKEN_LENGTH = 32

func MakeWHMCS(app *lobster.Lobster, ip string, secret string) *WHMCS {
	this := new(WHMCS)
	this.ip = ip
	this.secret = secret
	app.RegisterHttpHandler("/whmcs_connector", app.GetDatabase().WrapHandler(this.handleConnector), true)
	app.RegisterHttpHandler("/whmcs_token", app.GetDatabase().WrapHandler(lobster.SessionWrap(this.handleToken)), false)
	return this
}

type WHMCS struct {
	ip string
	secret string
	app *lobster.Lobster
}

func (this *WHMCS) handleConnector(w http.ResponseWriter, r *http.Request, db *lobster.Database) {
	r.ParseForm()
	if lobster.ExtractIP(r.RemoteAddr) != this.ip || r.PostForm.Get("secret") != this.secret {
		w.WriteHeader(403)
		return
	}

	switch r.PostForm.Get("action") {
	case "register":
		email := r.PostForm.Get("email")
		userId, err := lobster.UserCreate(db, email, utils.Uid(16), email)
		if err != nil {
			log.Printf("Failed to register account via WHMCS: %s (email=%s)", err.Error(), email)
			http.Error(w, err.Error(), 400)
		} else {
			log.Printf("Registered account via WHMCS (email=%s)", email)
			w.Write([]byte(fmt.Sprintf("%d", userId)))
		}
	case "token":
		userId, err := strconv.ParseInt(r.PostForm.Get("user_id"), 10, 64)
		if err != nil {
			http.Error(w, err.Error(), 400)
			return
		}

		token := utils.Uid(TOKEN_LENGTH)
		db.Exec("DELETE FROM whmcs_tokens WHERE time < DATE_SUB(NOW(), INTERVAL 1 MINUTE)")
		db.Exec("INSERT INTO whmcs_tokens (user_id, token) VALUES (?, ?)", userId, token)
		w.Write([]byte(token))
	default:
		http.Error(w, "unknown action", 400)
	}
}

func (this *WHMCS) handleToken(w http.ResponseWriter, r *http.Request, db *lobster.Database, session *lobster.Session) {
	if session.IsLoggedIn() {
		lobster.RedirectMessage(w, r, "/panel/dashboard", lobster.L.Info("already_logged_in"))
		return
	}

	r.ParseForm()
	token := r.Form.Get("token")
	if len(token) != TOKEN_LENGTH {
		http.Error(w, "bad token", 403)
	}
	rows := db.Query("SELECT id, user_id FROM whmcs_tokens WHERE token = ? AND time > DATE_SUB(NOW(), INTERVAL 1 MINUTE)", token)
	if !rows.Next() {
		http.Error(w, "invalid token", 403)
	}
	var rowId, userId int
	rows.Scan(&rowId, &userId)
	rows.Close()
	db.Exec("DELETE FROM whmcs_tokens WHERE id = ?", rowId)
	session.UserId = userId // we do not grant admin privileges on the session for WHMCS login
	log.Printf("Authentication via WHMCS for user_id=%d (%s)", userId, r.RemoteAddr)
	lobster.LogAction(db, userId, lobster.ExtractIP(r.RemoteAddr), "Logged in via WHMCS", "")
	http.Redirect(w, r, "/panel/dashboard", 303)

}
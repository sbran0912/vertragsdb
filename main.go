package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/gorilla/mux"
	"golang.org/x/crypto/bcrypt"
	_ "modernc.org/sqlite"
)

var db *sql.DB
var jwtSecret = []byte("your-secret-key-change-in-production")

type User struct {
	ID       int    `json:"id"`
	Username string `json:"username"`
	Password string `json:"-"`
	Role     string `json:"role"` // admin or viewer
}

type Contract struct {
	ID                     int        `json:"id"`
	ContractNumber         string     `json:"contract_number"`
	Title                  string     `json:"title"`
	Content                string     `json:"content"`
	Conditions             string     `json:"conditions"`
	NoticePeriod           *int       `json:"notice_period"`            // Kündigungsfrist in Monaten
	MinimumTerm            *time.Time `json:"minimum_term"`             // Mindestlaufzeit bis (Datum)
	TermMonths             *int       `json:"term_months"`              // Laufzeit in Monaten
	CancellationDate       *time.Time `json:"cancellation_date"`        // Berechneter Kündigungstermin
	CancellationActionDate *time.Time `json:"cancellation_action_date"` // Berechnete Kündigungsvornahme
	ValidFrom              time.Time  `json:"valid_from"`
	ValidUntil             *time.Time `json:"valid_until"`
	Partner                string     `json:"partner"`
	Category               string     `json:"category"`
	ContractType           string     `json:"contract_type"` // framework or individual
	FrameworkContractID    *int       `json:"framework_contract_id"`
	IsTerminated           bool       `json:"is_terminated"`
	TerminatedAt           *time.Time `json:"terminated_at"`
	CreatedAt              time.Time  `json:"created_at"`
}

type Document struct {
	ID         int       `json:"id"`
	ContractID int       `json:"contract_id"`
	Filename   string    `json:"filename"`
	FilePath   string    `json:"file_path"`
	UploadedAt time.Time `json:"uploaded_at"`
}

type Category struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

type Claims struct {
	UserID   int    `json:"user_id"`
	Username string `json:"username"`
	Role     string `json:"role"`
	jwt.RegisteredClaims
}

func initDB() error {
	var err error
	db, err = sql.Open("sqlite", "./contracts.db")
	if err != nil {
		return err
	}

	schema := `
	CREATE TABLE IF NOT EXISTS users (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		username TEXT UNIQUE NOT NULL,
		password TEXT NOT NULL,
		role TEXT NOT NULL CHECK(role IN ('admin', 'viewer'))
	);

	CREATE TABLE IF NOT EXISTS contracts (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		contract_number TEXT UNIQUE NOT NULL,
		title TEXT NOT NULL,
		content TEXT,
		conditions TEXT,
		notice_period INTEGER,
		minimum_term DATE,
		term_months INTEGER,
		cancellation_date DATE,
		cancellation_action_date DATE,
		valid_from DATETIME NOT NULL,
		valid_until DATETIME,
		partner TEXT NOT NULL,
		category TEXT NOT NULL,
		contract_type TEXT NOT NULL CHECK(contract_type IN ('framework', 'individual')),
		framework_contract_id INTEGER,
		is_terminated BOOLEAN DEFAULT 0,
		terminated_at DATETIME,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (framework_contract_id) REFERENCES contracts(id)
	);

	CREATE TABLE IF NOT EXISTS documents (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		contract_id INTEGER NOT NULL,
		filename TEXT NOT NULL,
		file_path TEXT NOT NULL,
		uploaded_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (contract_id) REFERENCES contracts(id)
	);

	CREATE TABLE IF NOT EXISTS categories (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT UNIQUE NOT NULL
	);
	`

	_, err = db.Exec(schema)
	if err != nil {
		return err
	}

	// Create default admin user (password: admin)
	hashedPassword, _ := bcrypt.GenerateFromPassword([]byte("admin"), bcrypt.DefaultCost)
	_, err = db.Exec("INSERT OR IGNORE INTO users (username, password, role) VALUES (?, ?, ?)",
		"admin", string(hashedPassword), "admin")
	if err != nil {
		return err
	}

	return migrateDB()
}

// migrateDB migriert das Schema falls nötig.
// Schema-Version 2: notice_period als INTEGER (Monate), minimum_term als DATE
func migrateDB() error {
	var version int
	db.QueryRow("PRAGMA user_version").Scan(&version)

	if version >= 4 {
		return nil
	}

	// Prüfe ob notice_period noch TEXT ist (alter Stand)
	rows, err := db.Query("PRAGMA table_info(contracts)")
	if err != nil {
		return err
	}
	needsMigration := false
	for rows.Next() {
		var cid int
		var name, dataType string
		var notNull int
		var dfltValue interface{}
		var pk int
		rows.Scan(&cid, &name, &dataType, &notNull, &dfltValue, &pk)
		if name == "notice_period" && dataType == "TEXT" {
			needsMigration = true
		}
	}
	rows.Close()

	if needsMigration {
		log.Println("Migriere contracts-Tabelle: notice_period TEXT→INTEGER, minimum_term TEXT→DATE")
		_, err = db.Exec(`
			CREATE TABLE contracts_v2 (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				contract_number TEXT UNIQUE NOT NULL,
				title TEXT NOT NULL,
				content TEXT,
				conditions TEXT,
				notice_period INTEGER,
				minimum_term DATE,
				valid_from DATETIME NOT NULL,
				valid_until DATETIME,
				partner TEXT NOT NULL,
				category TEXT NOT NULL,
				contract_type TEXT NOT NULL CHECK(contract_type IN ('framework', 'individual')),
				framework_contract_id INTEGER,
				is_terminated BOOLEAN DEFAULT 0,
				terminated_at DATETIME,
				created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
				FOREIGN KEY (framework_contract_id) REFERENCES contracts_v2(id)
			)`)
		if err != nil {
			return fmt.Errorf("migration create table: %w", err)
		}

		// Daten übertragen; notice_period: führende Zahl extrahieren (z.B. "3 Monate" → 3)
		// minimum_term: kann nicht automatisch konvertiert werden → NULL
		_, err = db.Exec(`
			INSERT INTO contracts_v2
				(id, contract_number, title, content, conditions,
				 notice_period, minimum_term, valid_from, valid_until,
				 partner, category, contract_type, framework_contract_id,
				 is_terminated, terminated_at, created_at)
			SELECT
				id, contract_number, title, content, conditions,
				NULLIF(CAST(notice_period AS INTEGER), 0),
				NULL,
				valid_from, valid_until,
				partner, category, contract_type, framework_contract_id,
				is_terminated, terminated_at, created_at
			FROM contracts`)
		if err != nil {
			return fmt.Errorf("migration copy data: %w", err)
		}

		_, err = db.Exec("DROP TABLE contracts")
		if err != nil {
			return fmt.Errorf("migration drop old table: %w", err)
		}

		_, err = db.Exec("ALTER TABLE contracts_v2 RENAME TO contracts")
		if err != nil {
			return fmt.Errorf("migration rename table: %w", err)
		}
		log.Println("Migration erfolgreich abgeschlossen")
	}

	_, err = db.Exec("PRAGMA user_version = 2")
	if err != nil {
		return err
	}
	version = 2

	// Migration v3: Neue Spalten term_months, cancellation_date, cancellation_action_date
	if version < 3 {
		for _, col := range []string{
			"ALTER TABLE contracts ADD COLUMN term_months INTEGER",
			"ALTER TABLE contracts ADD COLUMN cancellation_date DATE",
			"ALTER TABLE contracts ADD COLUMN cancellation_action_date DATE",
		} {
			db.Exec(col) // Fehler ignorieren falls Spalte schon existiert
		}
		_, err = db.Exec("PRAGMA user_version = 3")
		if err != nil {
			return err
		}
		version = 3
	}

	// Migration v4: Kategorien-Tabelle
	if version < 4 {
		_, err = db.Exec(`CREATE TABLE IF NOT EXISTS categories (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT UNIQUE NOT NULL
		)`)
		if err != nil {
			return fmt.Errorf("migration v4 create categories: %w", err)
		}

		for _, cat := range []string{"IT", "Gebäude", "Versicherungen"} {
			db.Exec("INSERT OR IGNORE INTO categories (name) VALUES (?)", cat)
		}

		// Kategorien aus bestehenden Verträgen übernehmen
		_, err = db.Exec(`INSERT OR IGNORE INTO categories (name)
			SELECT DISTINCT category FROM contracts
			WHERE category NOT IN ('IT', 'Gebäude', 'Versicherungen') AND category != ''`)
		if err != nil {
			return fmt.Errorf("migration v4 seed from contracts: %w", err)
		}

		_, err = db.Exec("PRAGMA user_version = 4")
		if err != nil {
			return err
		}
	}

	return nil
}

func generateToken(user User) (string, error) {
	claims := Claims{
		UserID:   user.ID,
		Username: user.Username,
		Role:     user.Role,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(24 * time.Hour)),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(jwtSecret)
}

func verifyToken(tokenString string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (interface{}, error) {
		return jwtSecret, nil
	})

	if err != nil {
		return nil, err
	}

	if claims, ok := token.Claims.(*Claims); ok && token.Valid {
		return claims, nil
	}

	return nil, fmt.Errorf("invalid token")
}

func authMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		tokenString := strings.TrimPrefix(authHeader, "Bearer ")
		claims, err := verifyToken(tokenString)
		if err != nil {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		r.Header.Set("X-User-ID", strconv.Itoa(claims.UserID))
		r.Header.Set("X-User-Role", claims.Role)
		next(w, r)
	}
}

func adminOnly(next http.HandlerFunc) http.HandlerFunc {
	return authMiddleware(func(w http.ResponseWriter, r *http.Request) {
		role := r.Header.Get("X-User-Role")
		if role != "admin" {
			http.Error(w, "Forbidden - Admin access required", http.StatusForbidden)
			return
		}
		next(w, r)
	})
}

func loginHandler(w http.ResponseWriter, r *http.Request) {
	var credentials struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}

	if err := json.NewDecoder(r.Body).Decode(&credentials); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	var user User
	err := db.QueryRow("SELECT id, username, password, role FROM users WHERE username = ?",
		credentials.Username).Scan(&user.ID, &user.Username, &user.Password, &user.Role)

	if err != nil {
		http.Error(w, "Invalid credentials", http.StatusUnauthorized)
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(credentials.Password)); err != nil {
		http.Error(w, "Invalid credentials", http.StatusUnauthorized)
		return
	}

	token, err := generateToken(user)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"token": token,
		"user": map[string]interface{}{
			"id":       user.ID,
			"username": user.Username,
			"role":     user.Role,
		},
	})
}

func createUserHandler(w http.ResponseWriter, r *http.Request) {
	var user User
	if err := json.NewDecoder(r.Body).Decode(&user); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(user.Password), bcrypt.DefaultCost)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	result, err := db.Exec("INSERT INTO users (username, password, role) VALUES (?, ?, ?)",
		user.Username, string(hashedPassword), user.Role)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	id, _ := result.LastInsertId()
	user.ID = int(id)
	user.Password = ""

	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(user)
}

func getUsersHandler(w http.ResponseWriter, r *http.Request) {
	rows, err := db.Query("SELECT id, username, role FROM users")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var users []User
	for rows.Next() {
		var user User
		if err := rows.Scan(&user.ID, &user.Username, &user.Role); err != nil {
			continue
		}
		users = append(users, user)
	}

	json.NewEncoder(w).Encode(users)
}

func updateUserHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]

	var input struct {
		Username string `json:"username"`
		Password string `json:"password"`
		Role     string `json:"role"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Letzten Admin nicht herabstufen
	if input.Role != "admin" {
		var adminCount int
		db.QueryRow("SELECT COUNT(*) FROM users WHERE role = 'admin' AND id != ?", id).Scan(&adminCount)
		if adminCount == 0 {
			http.Error(w, "Der letzte Admin kann nicht herabgestuft werden", http.StatusBadRequest)
			return
		}
	}

	var err error
	if input.Password != "" {
		hashedPassword, hashErr := bcrypt.GenerateFromPassword([]byte(input.Password), bcrypt.DefaultCost)
		if hashErr != nil {
			http.Error(w, hashErr.Error(), http.StatusInternalServerError)
			return
		}
		_, err = db.Exec("UPDATE users SET username = ?, password = ?, role = ? WHERE id = ?",
			input.Username, string(hashedPassword), input.Role, id)
	} else {
		_, err = db.Exec("UPDATE users SET username = ?, role = ? WHERE id = ?",
			input.Username, input.Role, id)
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	var user User
	db.QueryRow("SELECT id, username, role FROM users WHERE id = ?", id).Scan(&user.ID, &user.Username, &user.Role)
	json.NewEncoder(w).Encode(user)
}

func deleteUserHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]

	// Selbstlöschung verhindern
	if r.Header.Get("X-User-ID") == id {
		http.Error(w, "Sie können Ihren eigenen Account nicht löschen", http.StatusBadRequest)
		return
	}

	// Letzten Admin nicht löschen
	var userRole string
	db.QueryRow("SELECT role FROM users WHERE id = ?", id).Scan(&userRole)
	if userRole == "admin" {
		var adminCount int
		db.QueryRow("SELECT COUNT(*) FROM users WHERE role = 'admin'").Scan(&adminCount)
		if adminCount <= 1 {
			http.Error(w, "Der letzte Admin kann nicht gelöscht werden", http.StatusBadRequest)
			return
		}
	}

	_, err := db.Exec("DELETE FROM users WHERE id = ?", id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func getNextContractNumber() (string, error) {
	var maxNumber int
	err := db.QueryRow("SELECT COALESCE(MAX(CAST(SUBSTR(contract_number, 2) AS INTEGER)), 0) FROM contracts WHERE contract_number LIKE 'V%'").Scan(&maxNumber)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("V%06d", maxNumber+1), nil
}

func createContractHandler(w http.ResponseWriter, r *http.Request) {
	var contract Contract
	if err := json.NewDecoder(r.Body).Decode(&contract); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if contract.ContractNumber == "" {
		number, err := getNextContractNumber()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		contract.ContractNumber = number
	}

	var frameworkID interface{}
	if contract.FrameworkContractID != nil {
		frameworkID = *contract.FrameworkContractID
	}
	var noticePeriod interface{}
	if contract.NoticePeriod != nil {
		noticePeriod = *contract.NoticePeriod
	}
	var minimumTerm interface{}
	if contract.MinimumTerm != nil {
		minimumTerm = *contract.MinimumTerm
	}
	var termMonths interface{}
	if contract.TermMonths != nil {
		termMonths = *contract.TermMonths
	}

	result, err := db.Exec(`INSERT INTO contracts
		(contract_number, title, content, conditions, notice_period, minimum_term,
		term_months, valid_from, valid_until, partner, category, contract_type, framework_contract_id)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		contract.ContractNumber, contract.Title, contract.Content, contract.Conditions,
		noticePeriod, minimumTerm, termMonths, contract.ValidFrom, contract.ValidUntil,
		contract.Partner, contract.Category, contract.ContractType, frameworkID)

	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	id, _ := result.LastInsertId()
	contract.ID = int(id)

	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(contract)
}

func updateContractHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]

	var contract Contract
	if err := json.NewDecoder(r.Body).Decode(&contract); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	var frameworkID interface{}
	if contract.FrameworkContractID != nil {
		frameworkID = *contract.FrameworkContractID
	}
	var noticePeriod interface{}
	if contract.NoticePeriod != nil {
		noticePeriod = *contract.NoticePeriod
	}
	var minimumTerm interface{}
	if contract.MinimumTerm != nil {
		minimumTerm = *contract.MinimumTerm
	}
	var termMonths interface{}
	if contract.TermMonths != nil {
		termMonths = *contract.TermMonths
	}

	_, err := db.Exec(`UPDATE contracts SET
		title = ?, content = ?, conditions = ?, notice_period = ?,
		minimum_term = ?, term_months = ?, valid_from = ?, valid_until = ?, partner = ?,
		category = ?, contract_type = ?, framework_contract_id = ?
		WHERE id = ?`,
		contract.Title, contract.Content, contract.Conditions, noticePeriod,
		minimumTerm, termMonths, contract.ValidFrom, contract.ValidUntil, contract.Partner,
		contract.Category, contract.ContractType, frameworkID, id)

	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	json.NewEncoder(w).Encode(contract)
}

func getContractsHandler(w http.ResponseWriter, r *http.Request) {
	query := "SELECT id, contract_number, title, content, conditions, notice_period, minimum_term, term_months, cancellation_date, cancellation_action_date, valid_from, valid_until, partner, category, contract_type, framework_contract_id, is_terminated, terminated_at, created_at FROM contracts WHERE 1=1"

	args := []interface{}{}

	if search := r.URL.Query().Get("search"); search != "" {
		query += " AND (title LIKE ? OR partner LIKE ? OR content LIKE ?)"
		searchParam := "%" + search + "%"
		args = append(args, searchParam, searchParam, searchParam)
	}

	if category := r.URL.Query().Get("category"); category != "" {
		query += " AND category = ?"
		args = append(args, category)
	}

	if onlyValid := r.URL.Query().Get("only_valid"); onlyValid == "true" {
		query += " AND is_terminated = 0 AND (valid_until IS NULL OR valid_until > datetime('now'))"
	}

	query += " ORDER BY created_at DESC"

	rows, err := db.Query(query, args...)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	contracts := scanContracts(rows)
	json.NewEncoder(w).Encode(contracts)
}

// scanContracts liest alle Zeilen aus einem Contracts-Query und gibt sie als Slice zurück.
func scanContracts(rows *sql.Rows) []Contract {
	var contracts []Contract
	for rows.Next() {
		var contract Contract
		var validUntil, minimumTerm, cancDate, cancActionDate, terminatedAt sql.NullTime
		var frameworkID, noticePeriod, termMonths sql.NullInt64

		if err := rows.Scan(&contract.ID, &contract.ContractNumber, &contract.Title,
			&contract.Content, &contract.Conditions, &noticePeriod,
			&minimumTerm, &termMonths, &cancDate, &cancActionDate,
			&contract.ValidFrom, &validUntil, &contract.Partner,
			&contract.Category, &contract.ContractType, &frameworkID,
			&contract.IsTerminated, &terminatedAt, &contract.CreatedAt); err != nil {
			continue
		}

		if validUntil.Valid {
			contract.ValidUntil = &validUntil.Time
		}
		if minimumTerm.Valid {
			contract.MinimumTerm = &minimumTerm.Time
		}
		if termMonths.Valid {
			n := int(termMonths.Int64)
			contract.TermMonths = &n
		}
		if cancDate.Valid {
			contract.CancellationDate = &cancDate.Time
		}
		if cancActionDate.Valid {
			contract.CancellationActionDate = &cancActionDate.Time
		}
		if noticePeriod.Valid {
			n := int(noticePeriod.Int64)
			contract.NoticePeriod = &n
		}
		if frameworkID.Valid {
			id := int(frameworkID.Int64)
			contract.FrameworkContractID = &id
		}
		if terminatedAt.Valid {
			contract.TerminatedAt = &terminatedAt.Time
		}

		contracts = append(contracts, contract)
	}
	return contracts
}

func getContractHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]

	var contract Contract
	var validUntil, minimumTerm, cancDate, cancActionDate, terminatedAt sql.NullTime
	var frameworkID, noticePeriod, termMonths sql.NullInt64

	err := db.QueryRow(`SELECT id, contract_number, title, content, conditions,
		notice_period, minimum_term, term_months, cancellation_date, cancellation_action_date,
		valid_from, valid_until, partner, category,
		contract_type, framework_contract_id, is_terminated, terminated_at, created_at
		FROM contracts WHERE id = ?`, id).Scan(
		&contract.ID, &contract.ContractNumber, &contract.Title, &contract.Content,
		&contract.Conditions, &noticePeriod, &minimumTerm, &termMonths, &cancDate, &cancActionDate,
		&contract.ValidFrom, &validUntil, &contract.Partner, &contract.Category,
		&contract.ContractType, &frameworkID, &contract.IsTerminated,
		&terminatedAt, &contract.CreatedAt)

	if err != nil {
		http.Error(w, "Contract not found", http.StatusNotFound)
		return
	}

	if validUntil.Valid {
		contract.ValidUntil = &validUntil.Time
	}
	if minimumTerm.Valid {
		contract.MinimumTerm = &minimumTerm.Time
	}
	if termMonths.Valid {
		n := int(termMonths.Int64)
		contract.TermMonths = &n
	}
	if cancDate.Valid {
		contract.CancellationDate = &cancDate.Time
	}
	if cancActionDate.Valid {
		contract.CancellationActionDate = &cancActionDate.Time
	}
	if noticePeriod.Valid {
		n := int(noticePeriod.Int64)
		contract.NoticePeriod = &n
	}
	if frameworkID.Valid {
		id := int(frameworkID.Int64)
		contract.FrameworkContractID = &id
	}
	if terminatedAt.Valid {
		contract.TerminatedAt = &terminatedAt.Time
	}

	json.NewEncoder(w).Encode(contract)
}

func terminateContractHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]

	now := time.Now()
	_, err := db.Exec("UPDATE contracts SET is_terminated = 1, terminated_at = ? WHERE id = ?", now, id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	json.NewEncoder(w).Encode(map[string]string{"message": "Contract terminated"})
}

func uploadDocumentHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	contractID := vars["id"]

	if err := r.ParseMultipartForm(10 << 20); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	file, handler, err := r.FormFile("document")
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	defer file.Close()

	uploadsDir := "./uploads"
	os.MkdirAll(uploadsDir, os.ModePerm)

	filename := fmt.Sprintf("%s_%s", time.Now().Format("20060102150405"), handler.Filename)
	filepath := filepath.Join(uploadsDir, filename)

	dst, err := os.Create(filepath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer dst.Close()

	if _, err := io.Copy(dst, file); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	result, err := db.Exec("INSERT INTO documents (contract_id, filename, file_path) VALUES (?, ?, ?)",
		contractID, handler.Filename, filepath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	id, _ := result.LastInsertId()
	doc := Document{
		ID:         int(id),
		ContractID: mustAtoi(contractID),
		Filename:   handler.Filename,
		FilePath:   filepath,
		UploadedAt: time.Now(),
	}

	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(doc)
}

func getDocumentsHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	contractID := vars["id"]

	rows, err := db.Query("SELECT id, contract_id, filename, file_path, uploaded_at FROM documents WHERE contract_id = ?", contractID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var documents []Document
	for rows.Next() {
		var doc Document
		if err := rows.Scan(&doc.ID, &doc.ContractID, &doc.Filename, &doc.FilePath, &doc.UploadedAt); err != nil {
			continue
		}
		documents = append(documents, doc)
	}

	json.NewEncoder(w).Encode(documents)
}

func downloadDocumentHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	docID := vars["docId"]

	var doc Document
	err := db.QueryRow("SELECT id, filename, file_path FROM documents WHERE id = ?", docID).
		Scan(&doc.ID, &doc.Filename, &doc.FilePath)

	if err != nil {
		http.Error(w, "Document not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%s", doc.Filename))
	w.Header().Set("Content-Type", "application/pdf")
	http.ServeFile(w, r, doc.FilePath)
}

func getExpiringContractsHandler(w http.ResponseWriter, r *http.Request) {
	days := 90
	if d := r.URL.Query().Get("days"); d != "" {
		if parsed, err := strconv.Atoi(d); err == nil && parsed > 0 {
			days = parsed
		}
	}

	// Zeige Verträge, bei denen die Kündigungsvornahme innerhalb des Vorlaufzeitraums liegt.
	query := `SELECT id, contract_number, title, content, conditions, notice_period,
		minimum_term, term_months, cancellation_date, cancellation_action_date,
		valid_from, valid_until, partner, category, contract_type,
		framework_contract_id, is_terminated, terminated_at, created_at
		FROM contracts
		WHERE is_terminated = 0
		AND cancellation_action_date IS NOT NULL
		AND cancellation_action_date BETWEEN date('now') AND date('now', '+' || ? || ' days')
		ORDER BY cancellation_action_date ASC`

	rows, err := db.Query(query, days)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	contracts := scanContracts(rows)
	json.NewEncoder(w).Encode(contracts)
}

// calculateCancellationDates berechnet Kündigungstermin und Kündigungsvornahme für alle Verträge.
func calculateCancellationDatesHandler(w http.ResponseWriter, r *http.Request) {
	rows, err := db.Query(`SELECT id, valid_from, notice_period, minimum_term, term_months
		FROM contracts WHERE is_terminated = 0`)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	type calcContract struct {
		id           int
		validFrom    time.Time
		noticePeriod sql.NullInt64
		termMonths   sql.NullInt64
		minimumTerm  sql.NullTime
	}

	var contracts []calcContract
	for rows.Next() {
		var c calcContract
		if err := rows.Scan(&c.id, &c.validFrom, &c.noticePeriod, &c.minimumTerm, &c.termMonths); err != nil {
			continue
		}
		contracts = append(contracts, c)
	}
	rows.Close()

	today := time.Now().Truncate(24 * time.Hour)
	updated := 0

	for _, c := range contracts {
		// Alle Felder müssen gesetzt sein
		if !c.noticePeriod.Valid || !c.minimumTerm.Valid || !c.termMonths.Valid || c.termMonths.Int64 <= 0 {
			db.Exec("UPDATE contracts SET cancellation_date = NULL, cancellation_action_date = NULL WHERE id = ?", c.id)
			continue
		}

		np := int(c.noticePeriod.Int64)
		tm := int(c.termMonths.Int64)
		minTerm := c.minimumTerm.Time

		// Schritt 1: Erste Periodengrenze >= Mindestlaufzeit
		termin := c.validFrom
		for termin.Before(minTerm) {
			termin = termin.AddDate(0, tm, 0)
		}

		// Schritt 2: Kündigungsvornahme muss in der Zukunft liegen
		for termin.AddDate(0, -np, 0).Before(today) {
			termin = termin.AddDate(0, tm, 0)
		}

		cancDate := termin
		cancActionDate := termin.AddDate(0, -np, 0)
		_, err = db.Exec("UPDATE contracts SET cancellation_date = ?, cancellation_action_date = ? WHERE id = ?",
			cancDate, cancActionDate, c.id)
		if err == nil {
			updated++
		}
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"message": fmt.Sprintf("Kündigungstermine für %d Verträge berechnet", updated),
		"updated": updated,
	})
}

// Category handlers

func getCategoriesHandler(w http.ResponseWriter, r *http.Request) {
	rows, err := db.Query("SELECT id, name FROM categories ORDER BY name")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var categories []Category
	for rows.Next() {
		var cat Category
		if err := rows.Scan(&cat.ID, &cat.Name); err != nil {
			continue
		}
		categories = append(categories, cat)
	}

	json.NewEncoder(w).Encode(categories)
}

func createCategoryHandler(w http.ResponseWriter, r *http.Request) {
	var cat Category
	if err := json.NewDecoder(r.Body).Decode(&cat); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	cat.Name = strings.TrimSpace(cat.Name)
	if cat.Name == "" {
		http.Error(w, "Kategoriename darf nicht leer sein", http.StatusBadRequest)
		return
	}

	result, err := db.Exec("INSERT INTO categories (name) VALUES (?)", cat.Name)
	if err != nil {
		http.Error(w, "Kategorie existiert bereits", http.StatusConflict)
		return
	}

	id, _ := result.LastInsertId()
	cat.ID = int(id)

	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(cat)
}

func updateCategoryHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]

	var cat Category
	if err := json.NewDecoder(r.Body).Decode(&cat); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	cat.Name = strings.TrimSpace(cat.Name)
	if cat.Name == "" {
		http.Error(w, "Kategoriename darf nicht leer sein", http.StatusBadRequest)
		return
	}

	var oldName string
	err := db.QueryRow("SELECT name FROM categories WHERE id = ?", id).Scan(&oldName)
	if err != nil {
		http.Error(w, "Kategorie nicht gefunden", http.StatusNotFound)
		return
	}

	_, err = db.Exec("UPDATE categories SET name = ? WHERE id = ?", cat.Name, id)
	if err != nil {
		http.Error(w, "Kategoriename existiert bereits", http.StatusConflict)
		return
	}

	// Verträge mit altem Namen aktualisieren
	db.Exec("UPDATE contracts SET category = ? WHERE category = ?", cat.Name, oldName)

	cat.ID = mustAtoi(id)
	json.NewEncoder(w).Encode(cat)
}

func deleteCategoryHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]

	var catName string
	err := db.QueryRow("SELECT name FROM categories WHERE id = ?", id).Scan(&catName)
	if err != nil {
		http.Error(w, "Kategorie nicht gefunden", http.StatusNotFound)
		return
	}

	var count int
	db.QueryRow("SELECT COUNT(*) FROM contracts WHERE category = ?", catName).Scan(&count)
	if count > 0 {
		http.Error(w, fmt.Sprintf("Kategorie wird von %d Vertrag/Verträgen verwendet", count), http.StatusConflict)
		return
	}

	_, err = db.Exec("DELETE FROM categories WHERE id = ?", id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func mustAtoi(s string) int {
	i, _ := strconv.Atoi(s)
	return i
}

func main() {
	if err := initDB(); err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	basePath := "/vertragsdb"
	r := mux.NewRouter()
	api := r.PathPrefix(basePath + "/api").Subrouter()

	// Public routes
	api.HandleFunc("/login", loginHandler).Methods("POST")

	// Protected routes
	api.HandleFunc("/users", authMiddleware(getUsersHandler)).Methods("GET")
	api.HandleFunc("/users", adminOnly(createUserHandler)).Methods("POST")
	api.HandleFunc("/users/{id}", adminOnly(updateUserHandler)).Methods("PUT")
	api.HandleFunc("/users/{id}", adminOnly(deleteUserHandler)).Methods("DELETE")

	// Contract routes
	api.HandleFunc("/contracts", authMiddleware(getContractsHandler)).Methods("GET")
	api.HandleFunc("/contracts", adminOnly(createContractHandler)).Methods("POST")
	api.HandleFunc("/contracts/{id}", authMiddleware(getContractHandler)).Methods("GET")
	api.HandleFunc("/contracts/{id}", adminOnly(updateContractHandler)).Methods("PUT")
	api.HandleFunc("/contracts/{id}/terminate", adminOnly(terminateContractHandler)).Methods("POST")

	// Document routes
	api.HandleFunc("/contracts/{id}/documents", authMiddleware(getDocumentsHandler)).Methods("GET")
	api.HandleFunc("/contracts/{id}/documents", adminOnly(uploadDocumentHandler)).Methods("POST")
	api.HandleFunc("/documents/{docId}/download", authMiddleware(downloadDocumentHandler)).Methods("GET")

	// Reporting routes
	api.HandleFunc("/reports/expiring", authMiddleware(getExpiringContractsHandler)).Methods("GET")
	api.HandleFunc("/contracts/calculate-dates", adminOnly(calculateCancellationDatesHandler)).Methods("POST")

	// Category routes
	api.HandleFunc("/categories", authMiddleware(getCategoriesHandler)).Methods("GET")
	api.HandleFunc("/categories", adminOnly(createCategoryHandler)).Methods("POST")
	api.HandleFunc("/categories/{id}", adminOnly(updateCategoryHandler)).Methods("PUT")
	api.HandleFunc("/categories/{id}", adminOnly(deleteCategoryHandler)).Methods("DELETE")

	// Serve frontend files
	r.PathPrefix(basePath + "/").Handler(
		http.StripPrefix(basePath, http.FileServer(http.Dir("frontend/dist"))))

	log.Println("Server starting on :8091")
	log.Fatal(http.ListenAndServe(":8091", r))
}

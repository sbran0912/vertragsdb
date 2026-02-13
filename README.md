# Vertragsdatenbank

Eine webbasierte Anwendung zur Verwaltung von Verträgen. Verträge können erfasst, bearbeitet und mit Dokumenten versehen werden. Berichte zeigen aktive Verträge sowie Verträge mit ablaufender Kündigungsfrist.

## Technologie-Stack

| Komponente | Technologie |
|---|---|
| Backend | Go 1.22+ |
| Datenbank | SQLite (`modernc.org/sqlite`) |
| Routing | `net/http` ServeMux (stdlib) |
| Authentifizierung | JWT (HS256) + bcrypt |
| Frontend | Vanilla JavaScript |
| Build-Tool | Vite 7 |

## Funktionsübersicht

- **Vertragsverwaltung** – Anlegen, Bearbeiten und Beenden von Verträgen
- **Rahmenverträge** – Einzelverträge können einem Rahmenvertrag zugeordnet werden
- **Dokumentenverwaltung** – PDF-Dokumente können je Vertrag hochgeladen und heruntergeladen werden
- **Benutzerverwaltung** – Anlegen, Bearbeiten (inkl. Passwortvergabe) und Löschen von Benutzern; Rollen `admin` (Lesen + Schreiben) und `viewer` (nur Lesen)
- **Kategorieverwaltung** – Vertragskategorien über die GUI anlegen, umbenennen und löschen
- **Berichte** – Alle gültigen Verträge; Verträge mit ablaufender Kündigungsfrist (Vorlaufzeit frei wählbar)
- **Einstellungen** – Kategorieverwaltung

## Projektstruktur

```
vertragsdb/
├── main.go               # Go-Backend: REST-API, Datenbankzugriff, Authentifizierung
├── contracts.db          # SQLite-Datenbank (wird beim ersten Start angelegt)
├── uploads/              # Hochgeladene PDF-Dokumente
├── go.mod / go.sum       # Go-Abhängigkeiten
└── frontend/
    ├── index.html        # Single-Page-Application (HTML-Gerüst)
    ├── src/
    │   ├── main.js       # Frontend-Logik (Vanilla JS)
    │   └── styles.css    # Stylesheet
    ├── dist/             # Produktions-Build (wird von Go ausgeliefert)
    ├── package.json
    └── vite.config.js
```

## Einrichtung und Start

### Voraussetzungen

- Go 1.22 oder neuer (getestet mit Go 1.25)
- Node.js 18 oder neuer (für den Frontend-Build)

### Frontend bauen

```bash
cd frontend
npm install
npm run build
```

### Backend starten

```bash
go run main.go
```

Die Anwendung ist anschließend unter **http://localhost:8091/vertragsdb/** erreichbar.

### Standard-Zugangsdaten

| Benutzername | Passwort | Rolle |
|---|---|---|
| `admin` | `admin` | Admin |

Das Passwort sollte nach dem ersten Login geändert werden.

## Datenmodell

### Benutzer (`users`)

| Feld | Typ | Beschreibung |
|---|---|---|
| `id` | INTEGER | Primärschlüssel |
| `username` | TEXT | Benutzername (eindeutig) |
| `password` | TEXT | Passwort-Hash (bcrypt) |
| `role` | TEXT | `admin` oder `viewer` |

### Vertrag (`contracts`)

| Feld | Typ | Beschreibung |
|---|---|---|
| `id` | INTEGER | Primärschlüssel |
| `contract_number` | TEXT | Eindeutige Vertragsnummer (automatisch: `V000001`, `V000002`, …) |
| `title` | TEXT | Vertragstitel |
| `partner` | TEXT | Vertragspartner |
| `category` | TEXT | Kategorie (dynamisch aus Tabelle `categories`) |
| `contract_type` | TEXT | `framework` (Rahmenvertrag) oder `individual` (Einzelvertrag) |
| `framework_contract_id` | INTEGER | Fremdschlüssel auf übergeordneten Rahmenvertrag (optional) |
| `valid_from` | DATETIME | Beginn der Vertragslaufzeit |
| `valid_until` | DATETIME | Ende der Vertragslaufzeit (optional) |
| `notice_period` | INTEGER | Kündigungsfrist in **Monaten** |
| `minimum_term` | DATE | Mindestlaufzeit bis (Datum) |
| `term_months` | INTEGER | Laufzeit in **Monaten** (Verlängerungsperiode) |
| `cancellation_date` | DATE | Nächster Kündigungstermin (berechnet) |
| `cancellation_action_date` | DATE | Kündigungsvornahme – spätester Handlungstag (berechnet) |
| `content` | TEXT | Vertragsinhalt (Freitext) |
| `conditions` | TEXT | Vertragskonditionen (Freitext) |
| `is_terminated` | BOOLEAN | Wurde der Vertrag manuell beendet? |
| `terminated_at` | DATETIME | Zeitpunkt der manuellen Beendigung |
| `created_at` | DATETIME | Anlagedatum |

### Dokumente (`documents`)

| Feld | Typ | Beschreibung |
|---|---|---|
| `id` | INTEGER | Primärschlüssel |
| `contract_id` | INTEGER | Fremdschlüssel auf `contracts` |
| `filename` | TEXT | Originaler Dateiname |
| `file_path` | TEXT | Pfad zur gespeicherten Datei im `uploads/`-Verzeichnis |
| `uploaded_at` | DATETIME | Upload-Zeitpunkt |

### Kategorien (`categories`)

| Feld | Typ | Beschreibung |
|---|---|---|
| `id` | INTEGER | Primärschlüssel (Auto-Increment) |
| `name` | TEXT | Kategoriename (eindeutig) |

Kategorien werden unter **Einstellungen → Kategorien verwalten** gepflegt. Beim Umbenennen einer Kategorie werden alle Verträge mit dem alten Namen automatisch aktualisiert. Eine Kategorie kann nur gelöscht werden, wenn sie von keinem Vertrag verwendet wird.

## REST-API

### Authentifizierung

Alle Endpunkte außer `/vertragsdb/api/login` erfordern einen JWT-Token im Header:

```
Authorization: Bearer <token>
```

### Endpunkte

Alle Pfade sind relativ zum Base-Path `/vertragsdb`.

| Methode | Pfad | Rolle | Beschreibung |
|---|---|---|---|
| `POST` | `/vertragsdb/api/login` | – | Anmelden, liefert JWT-Token |
| `GET` | `/vertragsdb/api/contracts` | viewer | Alle Verträge (Filter: `search`, `category`, `only_valid`) |
| `POST` | `/vertragsdb/api/contracts` | admin | Neuen Vertrag anlegen |
| `GET` | `/vertragsdb/api/contracts/{id}` | viewer | Einzelnen Vertrag abrufen |
| `PUT` | `/vertragsdb/api/contracts/{id}` | admin | Vertrag aktualisieren |
| `POST` | `/vertragsdb/api/contracts/{id}/terminate` | admin | Vertrag beenden |
| `GET` | `/vertragsdb/api/contracts/{id}/documents` | viewer | Dokumente eines Vertrags |
| `POST` | `/vertragsdb/api/contracts/{id}/documents` | admin | Dokument hochladen (PDF, max. 10 MB) |
| `GET` | `/vertragsdb/api/documents/{docId}/download` | viewer | Dokument herunterladen |
| `GET` | `/vertragsdb/api/reports/expiring?days=90` | viewer | Verträge mit ablaufender Kündigungsfrist (Standard: 90 Tage) |
| `POST` | `/vertragsdb/api/contracts/calculate-dates` | admin | Kündigungstermine für alle Verträge berechnen |
| `GET` | `/vertragsdb/api/users` | viewer | Alle Benutzer |
| `POST` | `/vertragsdb/api/users` | admin | Neuen Benutzer anlegen |
| `PUT` | `/vertragsdb/api/users/{id}` | admin | Benutzer bearbeiten (Benutzername, Rolle, Passwort optional) |
| `DELETE` | `/vertragsdb/api/users/{id}` | admin | Benutzer löschen |
| `GET` | `/vertragsdb/api/categories` | viewer | Alle Kategorien abrufen |
| `POST` | `/vertragsdb/api/categories` | admin | Neue Kategorie anlegen |
| `PUT` | `/vertragsdb/api/categories/{id}` | admin | Kategorie umbenennen (kaskadiert auf Verträge) |
| `DELETE` | `/vertragsdb/api/categories/{id}` | admin | Kategorie löschen (nur wenn unbenutzt) |

## Benutzerverwaltung

Admins können Benutzer anlegen, bearbeiten und löschen. Beim Bearbeiten kann das Passwort leer gelassen werden – in diesem Fall bleibt das bestehende Passwort erhalten.

Folgende Schutzmechanismen sind serverseitig erzwungen:

| Situation | Verhalten |
|---|---|
| Eigenen Account löschen | Nicht erlaubt |
| Letzten Admin löschen | Nicht erlaubt |
| Letzten Admin zum Viewer herabstufen | Nicht erlaubt |

## Berechnung: Kündigungstermin und Kündigungsvornahme

Per Button „Kündigungstermine berechnen" auf der Berichte-Seite werden für alle nicht-beendeten Verträge die Felder `cancellation_date` und `cancellation_action_date` berechnet und in der Datenbank gespeichert.

### Eingangswerte

| Feld | Beschreibung |
|---|---|
| `valid_from` | Vertragsbeginn |
| `minimum_term` | Mindestlaufzeit bis (Datum) |
| `term_months` | Laufzeit / Verlängerungsperiode (Monate) |
| `notice_period` | Kündigungsfrist (Monate) |

Alle vier Felder müssen gesetzt sein. Fehlt ein Wert, bleiben die berechneten Felder `NULL`.

### Algorithmus

```
Schritt 1: Nächste Periodengrenze ab Mindestlaufzeit finden
  Termin = valid_from
  SOLANGE Termin < minimum_term:
    Termin = Termin + term_months Monate

Schritt 2: Prüfe ob Kündigungsvornahme noch in der Zukunft liegt
  SOLANGE (Termin − notice_period Monate) < Heute:
    Termin = Termin + term_months Monate

Ergebnis:
  Kündigungstermin    = Termin
  Kündigungsvornahme  = Termin − notice_period Monate
```

### Beispiel

| Eingabe | Wert |
|---|---|
| Gültig ab | 01.01.2024 |
| Mindestlaufzeit bis | 01.01.2026 |
| Laufzeit | 12 Monate |
| Kündigungsfrist | 3 Monate |
| Heute | 12.02.2026 |

Perioden ab Vertragsbeginn: 01.01.2025 → 01.01.2026 → 01.01.2027 → ...

1. Erste Grenze ≥ Mindestlaufzeit: **01.01.2026**
2. Kündigungsvornahme wäre 01.10.2025 → liegt in der Vergangenheit → nächste Periode
3. Nächste Grenze: **01.01.2027**, Kündigungsvornahme = 01.10.2026 → liegt in der Zukunft

**Ergebnis: Kündigungstermin = 01.01.2027, Kündigungsvornahme = 01.10.2026**

## Bericht: Ablaufende Kündigungsfrist

Der Bericht zeigt Verträge, bei denen **jetzt Handlungsbedarf** besteht – also Verträge, deren Kündigungsvornahme innerhalb des konfigurierten Vorlaufzeitraums liegt.

Ein Vertrag erscheint im Bericht, wenn gilt:
```
Heute ≤ Kündigungsvornahme ≤ Heute + Vorlaufzeit
```

Die Vorlaufzeit wird direkt neben dem Button im Bericht eingegeben (Standard: 90 Tage).

Voraussetzungen:
- Vertrag ist nicht manuell beendet
- Kündigungstermine wurden berechnet (`cancellation_action_date` ist gesetzt)

## Datenbankmigrationen

Die Anwendung verwaltet das Datenbankschema selbst. Beim Start wird geprüft, ob eine Migration notwendig ist (`PRAGMA user_version`).

| Version | Änderung |
|---|---|
| 2 | `notice_period`: TEXT → INTEGER (Monate); `minimum_term`: TEXT → DATE. Vorhandene Textwerte wie „3 Monate" werden automatisch zu `3` migriert. `minimum_term`-Textwerte werden auf `NULL` gesetzt und müssen manuell neu eingetragen werden. |
| 3 | Neue Spalten: `term_months` (INTEGER), `cancellation_date` (DATE), `cancellation_action_date` (DATE). |
| 4 | Neue Tabelle `categories` mit Seed der bestehenden Kategorien (IT, Gebäude, Versicherungen) sowie aller bereits in Verträgen genutzten Kategoriewerte. |

## Entwicklung

### Frontend im Entwicklungsmodus

```bash
cd frontend
npm run dev
```

Vite startet einen Dev-Server mit Hot-Module-Replacement. Das Backend muss separat laufen; Vite leitet API-Anfragen an `/vertragsdb/api` automatisch an `http://localhost:8091` weiter (Proxy-Konfiguration in `vite.config.js`).

### Produktions-Build

```bash
cd frontend && npm run build
go build -o vertragsdb .
./vertragsdb
```

## Sicherheitshinweise

- Den JWT-Secret in `main.go` (`jwtSecret`) vor dem produktiven Einsatz durch einen sicheren Zufallswert ersetzen.
- Das Standard-Passwort `admin` nach dem ersten Login ändern.
- HTTPS sollte über einen vorgelagerten Reverse-Proxy (z. B. nginx) bereitgestellt werden.
- Hochgeladene Dateien werden im Verzeichnis `uploads/` gespeichert und sollten in ein Backup einbezogen werden.

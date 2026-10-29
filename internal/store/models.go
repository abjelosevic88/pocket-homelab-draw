package store

// User is an account on this box.
type User struct {
	ID           int64  `json:"id"`
	Email        string `json:"email"`
	Name         string `json:"name"`
	PasswordHash string `json:"-"`
	Role         string `json:"role"` // admin | member
	CreatedAt    string `json:"created_at"`
}

// APIToken is a personal access token (hash stored, plaintext shown once).
type APIToken struct {
	ID         int64   `json:"id"`
	UserID     int64   `json:"user_id"`
	Name       string  `json:"name"`
	Prefix     string  `json:"prefix"`
	LastUsedAt *string `json:"last_used_at"`
	CreatedAt  string  `json:"created_at"`
}

// Kinds of drawing.
const (
	KindSketch  = "sketch"  // Excalidraw: a hand-drawn whiteboard
	KindMermaid = "mermaid" // a diagram written as text
	KindDrawio  = "drawio"  // a draw.io diagram
)

// ValidKind reports whether k is a kind of drawing.
func ValidKind(k string) bool { return k == KindSketch || k == KindMermaid || k == KindDrawio }

// Folder groups drawings.
type Folder struct {
	ID        int64  `json:"id"`
	UserID    int64  `json:"-"`
	Name      string `json:"name"`
	Sort      int    `json:"sort"`
	CreatedAt string `json:"created_at"`
	Count     int    `json:"count"`
}

// Drawing is one sketch, text diagram or draw.io diagram. Data and Preview
// are left empty by list queries.
type Drawing struct {
	ID         int64  `json:"id"`
	UserID     int64  `json:"-"`
	UID        string `json:"uid"`
	FolderID   *int64 `json:"folder_id"`
	Kind       string `json:"kind"`
	Title      string `json:"title"`
	Data       string `json:"data,omitempty"`
	Preview    string `json:"-"`
	PreviewRev int    `json:"preview_rev"`
	SearchText string `json:"-"`
	Starred    bool   `json:"starred"`
	Rev        int    `json:"rev"`
	Bytes      int    `json:"bytes"`
	Source     string `json:"source"`
	FilesPath  string `json:"files_path"`
	FilesRev   int    `json:"files_rev"`
	FilesError string `json:"files_error"`
	OpenedAt   string `json:"opened_at"`
	EditedAt   string `json:"edited_at"`
	TrashedAt  string `json:"trashed_at"`
	CreatedAt  string `json:"created_at"`
	UpdatedAt  string `json:"updated_at"`

	// Filled by queries.
	FolderName string `json:"folder_name"`
	HasPreview bool   `json:"has_preview"`
	Versions   int    `json:"versions"`
}

// Version is an earlier state of a drawing.
type Version struct {
	ID        int64  `json:"id"`
	UserID    int64  `json:"-"`
	DrawingID int64  `json:"drawing_id"`
	Rev       int    `json:"rev"`
	Title     string `json:"title"`
	Data      string `json:"data,omitempty"`
	Preview   string `json:"-"`
	Bytes     int    `json:"bytes"`
	Reason    string `json:"reason"`
	CreatedAt string `json:"created_at"`
}

// Settings are the box-wide values plus one person's preferences and their
// own connections. Connection modes: "core" (an app link through Pocket
// Core), "direct" (URL + token) or "off"; "" = not chosen yet.
type Settings struct {
	Title         string `json:"title"`
	SetupComplete bool   `json:"setup_complete"`
	// DrawioURL is the draw.io server diagrams open in (box-wide, admin).
	DrawioURL string `json:"drawio_url"`

	DateFormat string `json:"date_format"`
	TimeFormat string `json:"time_format"`
	WeekStart  int    `json:"week_start"` // 1 = Monday, 7 = Sunday

	// Drawing.
	DefaultKind  string `json:"default_kind"`  // what + makes: sketch | mermaid | drawio | ask
	SketchGrid   bool   `json:"sketch_grid"`   // new sketches start with the grid on
	SketchFont   string `json:"sketch_font"`   // hand | normal | code
	SketchSnap   bool   `json:"sketch_snap"`   // shapes snap to each other
	MermaidTheme string `json:"mermaid_theme"` // auto | default | neutral | forest | dark
	MermaidLook  string `json:"mermaid_look"`  // classic | handDrawn
	ListStyle    string `json:"list_style"`    // grid | list
	SortBy       string `json:"sort_by"`       // edited | created | title
	TrashDays    int    `json:"trash_days"`    // trashed drawings go after; 0 = keep
	KeepVersions int    `json:"keep_versions"` // per drawing

	// Notifications.
	NotifyURL      string `json:"notify_url"`
	NotifyFormat   string `json:"notify_format"`       // apprise | ntfy | json
	NotifyLinkAdds bool   `json:"notify_link_adds"`    // another app added a drawing
	NotifyFiles    bool   `json:"notify_files_errors"` // copying to Pocket Files failed

	// Pocket Files: a copy of each drawing (picture + file) in a folder.
	FilesMode      string `json:"files_mode"`
	FilesURL       string `json:"files_url"`
	FilesToken     string `json:"files_token"`
	FilesPublicURL string `json:"files_public_url"`
	FilesFolder    string `json:"files_folder"`
	FilesAuto      bool   `json:"files_auto"` // keep the copies up to date by themselves

	// Pocket Notes and Pocket Projects: which notes and issues show a drawing
	// ("Used in"), read only.
	NotesMode         string `json:"notes_mode"`
	NotesURL          string `json:"notes_url"`
	NotesToken        string `json:"notes_token"`
	NotesPublicURL    string `json:"notes_public_url"`
	ProjectsMode      string `json:"projects_mode"`
	ProjectsURL       string `json:"projects_url"`
	ProjectsToken     string `json:"projects_token"`
	ProjectsPublicURL string `json:"projects_public_url"`

	// AI for drafting diagrams: Pocket AI through Core, or an
	// OpenAI-compatible API with the person's own key.
	AIProvider    string `json:"ai_provider"` // auto | pocket | openai
	AIPocketModel string `json:"ai_pocket_model"`
	OpenAIKey     string `json:"openai_api_key"`
	OpenAIModel   string `json:"openai_model"`
	OpenAIBaseURL string `json:"openai_base_url"`
	AILanguage    string `json:"ai_language"` // labels' language, "" = the request's
}

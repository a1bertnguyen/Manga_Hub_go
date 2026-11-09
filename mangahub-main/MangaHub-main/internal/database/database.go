package database

import (
	"database/sql"
	"log"
	"os"
	"mangahub/pkg/models"

	_ "modernc.org/sqlite"
)

// DB is the global database instance
var DB *sql.DB

// Initialize sets up the database connection and creates tables
// This function must be called before any database operations.
// It sets the global database.DB variable.
func Initialize(dbPath string) error {
	// Create directory if not exists
	if err := os.MkdirAll("./data", os.ModePerm); err != nil {
		return err
	}

	var err error
	DB, err = sql.Open("sqlite", dbPath)
	if err != nil {
		return err
	}

	// Test connection
	if err = DB.Ping(); err != nil {
		return err
	}

	log.Println("Database connected successfully!")

	// Create tables
	if err = createTables(); err != nil {
		return err
	}

	return nil
}

// createTables creates all required database tables
func createTables() error {
	queries := []string{
		`CREATE TABLE IF NOT EXISTS users (
			id TEXT PRIMARY KEY,
			username TEXT UNIQUE NOT NULL,
			email TEXT UNIQUE NOT NULL,
			password_hash TEXT NOT NULL,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS manga (
			id TEXT PRIMARY KEY,
			title TEXT NOT NULL,
			author TEXT,
			genres TEXT,
			status TEXT,
			total_chapters INTEGER,
			description TEXT
		)`,
		`CREATE TABLE IF NOT EXISTS user_progress (
			user_id TEXT,
			manga_id TEXT,
			current_chapter INTEGER DEFAULT 0,
			status TEXT DEFAULT 'reading',
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			PRIMARY KEY (user_id, manga_id),
			FOREIGN KEY (user_id) REFERENCES users(id),
			FOREIGN KEY (manga_id) REFERENCES manga(id)
		)`,
	}

	for _, query := range queries {
		if _, err := DB.Exec(query); err != nil {
			return err
		}
	}

	log.Println("Database tables created successfully!")
	return nil
}

// Seed manga table with data
func SeedManga() error {
	mangas := []models.Manga{
		{ID:"one-piece", Title:"One Piece", Author:"Eiichiro Oda", Genres:[]string{"Action", "Adventure", "Shounen"}, Status:"ongoing", TotalChapters:1100, Description:"Pirate adventure"},
		{ID:"demon-slayer", Title:"Demon Slayer: Kimetsu no Yaiba", Author:"Koyoharu Gotouge", Genres:[]string{"Action","Supernatural","Adventure"}, Status:"completed", TotalChapters:205, Description:"Tanjiro fights demons after his family is slaughtered."},
		{ID:"attack-on-titan", Title:"Attack on Titan", Author:"Hajime Isayama", Genres:[]string{"Action", "Drama", "Mystery"}, Status:"completed", TotalChapters:139, Description:"Humanity fights for survival against giant Titans.",},
		{ID:"naruto", Title:"Naruto", Author:"Masashi Kishimoto", Genres: []string{"Action", "Shounen"}, Status:"completed", TotalChapters:700, Description:"Ninja story"},
		{ID:"dragon-ball", Title:"Dragon Ball", Author:"Akira Toriyama", Genres:[]string{"Action","Adventure","Martial Arts"}, Status:"completed", TotalChapters:520, Description:"Goku grows up training in martial arts and seeks the Dragon Balls."},
		{ID:"detective-conan", Title:"Detective Conan", Author:"Gosho Aoyama", Genres:[]string{"Mystery","Crime","Adventure"}, Status:"ongoing", TotalChapters:1000, Description:"Teen detective is shrunk to a child and solves mysteries while searching for a cure."},
		{ID:"slamdunk", Title:"Slam Dunk", Author:"Takehiko Inoue", Genres:[]string{"Sports","Comedy","Drama"}, Status:"completed", TotalChapters:276, Description:"A delinquent joins his high school basketball team and discovers passion for the sport."},
		{ID:"haikyuu", Title:"Haikyuu!!", Author:"Haruichi Furudate", Genres:[]string{"Sports","Comedy","Drama"}, Status:"completed", TotalChapters:402, Description:"A short volleyball player aims to lead his team to nationals."},
		{ID:"hunter-x-hunter", Title:"Hunter x Hunter", Author:"Yoshihiro Togashi", Genres:[]string{"Adventure","Action","Fantasy"}, Status:"hiatus", TotalChapters:390, Description:"Gon becomes a Hunter to find his father and encounters dangerous challenges."},
		{ID:"fullmetal-alchemist", Title:"Fullmetal Alchemist", Author:"Hiromu Arakawa", Genres:[]string{"Action","Adventure","Fantasy"}, Status:"completed", TotalChapters:116, Description:"Two brothers use alchemy in a quest to restore their bodies after a tragic accident."},
		{ID:"jojos-bizarre-adventure", Title:"JoJo Bizarre Adventure", Author:"Hirohiko Araki", Genres:[]string{"Action","Adventure","Supernatural"}, Status:"ongoing", TotalChapters:1400, Description:"Generational saga of the Joestar family and their battles with bizarre forces."},
        {ID:"world-trigger", Title:"World Trigger", Author:"Daisuke Ashihara", Genres:[]string{"Action","Sci-Fi","Adventure"}, Status:"ongoing", TotalChapters:300, Description:"Agents defend Earth from interdimensional beings using strategic battles."},
        {ID:"gintama", Title:"Gintama", Author:"Hideaki Sorachi", Genres:[]string{"Action","Comedy","Sci-Fi"}, Status:"completed", TotalChapters:704, Description:"A samurai freelancer takes odd jobs in an alien-occupied Edo."},
        {ID:"bleach", Title:"Bleach", Author:"Tite Kubo", Genres:[]string{"Action","Adventure","Supernatural"}, Status:"completed", TotalChapters:686, Description:"A teenager becomes a Soul Reaper and battles evil spirits."}, 
        {ID:"yu-yu-hakusho", Title:"Yu Yu Hakusho", Author:"Yoshihiro Togashi", Genres:[]string{"Action","Supernatural","Comedy"}, Status:"completed", TotalChapters:175, Description:"A teen becomes a Spirit Detective after dying and returning to life."}, 
        {ID:"kingdom", Title:"Kingdom", Author:"Yasuhisa Hara", Genres:[]string{"Historical","Action","War"}, Status:"ongoing", TotalChapters:800, Description:"Two boys rise through the ranks of ancient China warring states to unify the kingdom."},
        {ID:"inuyasha", Title:"Inuyasha", Author:"Rumiko Takahashi", Genres:[]string{"Adventure","Fantasy","Romance"}, Status:"completed", TotalChapters:558, Description:"A schoolgirl travels to the Sengoku era and teams with a half-demon to collect shards of a sacred jewel."},
        {ID:"ashita-no-joe", Title:"Ashita no Joe", Author:"Asao Takamori & Tetsuya Chiba", Genres:[]string{"Sports","Drama"}, Status:"completed", TotalChapters:170, Description:"An underdog boxer rises through hardships in the ring and life."},
        {ID:"dragon-quest-dai", Title:"Dragon Quest: The Adventure of Dai", Author:"Riku Sanjo & Koji Inada", Genres:[]string{"Action","Adventure","Fantasy"}, Status:"completed", TotalChapters:370, Description:"A young hero fights dark forces in a classic RPG-style world."},
		{ID:"death-note", Title:"Death Note", Author:"Tsugumi Ohba & Takeshi Obata", Genres:[]string{"Mystery","Psychological","Supernatural"}, Status:"completed", TotalChapters:108, Description:"A notebook that kills anyone whose name is written in it sparks a cat-and-mouse battle."}, 
        {ID:"berserk", Title:"Berserk", Author:"Kentaro Miura", Genres:[]string{"Dark Fantasy","Action"}, Status:"completed (posthumous)", TotalChapters:360, Description:"A lone mercenary with a tragic past battles demons in a brutal dark world."},
        {ID:"tokyo-ghoul", Title:"Tokyo Ghoul", Author:"Sui Ishida", Genres:[]string{"Horror","Action","Supernatural"}, Status:"completed", TotalChapters:144, Description:"A college student becomes half-ghoul and must survive in a violent world."},
        {ID:"the-promised-neverland", Title:"The Promised Neverland", Author:"Kaiu Shirai & Posuka Demizu", Genres:[]string{"Mystery","Thriller","Sci-Fi"}, Status:"completed", TotalChapters:181, Description:"Orphaned kids uncover terrifying secrets about their home."},
        {ID:"mob-psycho-100", Title:"Mob Psycho 100", Author:"ONE", Genres:[]string{"Action","Comedy","Supernatural"}, Status:"completed", TotalChapters:100, Description:"A powerful psychic teen tries to live a normal life."},
        {ID:"rorouni-kenshin", Title:"Rurouni Kenshin", Author:"Nobuhiro Watsuki", Genres:[]string{"Action","Historical","Romance"}, Status:"completed", TotalChapters:255, Description:"A wandering swordsman seeks atonement after a violent past."},
        {ID:"made-in-abyss", Title:"Made in Abyss", Author:"Akihito Tsukushi", Genres:[]string{"Adventure","Fantasy","Mystery"}, Status:"ongoing", TotalChapters:80, Description:"Explorers descend into a dangerous abyss with strange creatures and relics."},
        {ID:"vinland-saga", Title:"Vinland Saga", Author:"Makoto Yukimura", Genres:[]string{"Historical","Action","Drama"}, Status:"ongoing", TotalChapters:200, Description:"A young warrior quest for revenge in Viking age Europe."},
        {ID:"chainsaw-man", Title:"Chainsaw Man", Author:"Tatsuki Fujimoto", Genres:[]string{"Action","Horror","Comedy"}, Status:"ongoing", TotalChapters:150, Description:"A devil-hunter bonded with a devil fights other devils."},
        {ID:"spy-x-family", Title:"Spy x Family", Author:"Tatsuya Endo", Genres:[]string{"Action","Comedy","Slice of Life"}, Status:"ongoing", TotalChapters:120, Description:"A spy must assemble a fake family for a mission — not knowing they all have secrets."},
		{ID:"dorohedoro", Title:"Dorohedoro", Author:"Q Hayashida", Genres:[]string{"Dark Fantasy","Action","Horror"}, Status:"completed", TotalChapters:167, Description:"In a grim, chaotic world, a man with a reptile head searches for the sorcerer who cursed him."},
        {ID:"20th-century-boys", Title:"20th Century Boys", Author:"Naoki Urasawa", Genres:[]string{"Mystery","Psychological","Sci-Fi"}, Status:"completed", TotalChapters:249, Description:"Childhood friends confront a mysterious cult whose leader may be linked to their past."},
        {ID:"black-clover", Title:"Black Clover", Author:"Yūki Tabata", Genres:[]string{"Fantasy","Action","Adventure"}, Status:"completed", TotalChapters:368, Description:"A boy born without magic strives to become the Wizard King in a world where magic is everything."},
        {ID:"jujutsu-kaisen", Title:"Jujutsu Kaisen", Author:"Gege Akutami", Genres:[]string{"Supernatural","Action","Dark Fantasy"}, Status:"completed", TotalChapters:271, Description:"A high school student becomes involved in the dangerous world of curses and jujutsu sorcerers."},
        {ID:"boku-no-hero-academia", Title:"My Hero Academia", Author:"Kohei Horikoshi", Genres:[]string{"Superhero","Action","Drama"}, Status:"completed", TotalChapters:430, Description:"In a world where most people have superpowers, a powerless boy dreams of becoming a hero."},
		{ID:"magi", Title:"Magi: The Labyrinth of Magic", Author:"Shinobu Ohtaka", Genres:[]string{"Adventure","Fantasy","Action"}, Status:"completed", TotalChapters:369, Description:"A magical adventure inspired by Arabian Nights following Aladdin and his allies."},
        {ID:"claymore", Title:"Claymore", Author:"Norihiro Yagi", Genres:[]string{"Dark Fantasy","Action","Horror"}, Status:"completed", TotalChapters:155, Description:"Female warriors battle monstrous Yoma while struggling with their own humanity."},
        {ID:"monster", Title:"Monster", Author:"Naoki Urasawa", Genres:[]string{"Mystery","Psychological","Thriller"}, Status:"completed", TotalChapters:162, Description:"A surgeon hunts a former patient who grew into a terrifying serial killer."},
        {ID:"vagabond", Title:"Vagabond", Author:"Takehiko Inoue", Genres:[]string{"Historical","Action","Drama"}, Status:"hiatus", TotalChapters:327, Description:"A philosophical retelling of the life of legendary swordsman Miyamoto Musashi."},
        {ID:"golden-kamuy", Title:"Golden Kamuy", Author:"Satoru Noda", Genres:[]string{"Historical","Action","Adventure"}, Status:"completed", TotalChapters:314, Description:"A treasure hunt in Hokkaido involving soldiers, criminals, and Ainu culture."},
        {ID:"parasyte", Title:"Parasyte", Author:"Hitoshi Iwaaki", Genres:[]string{"Horror","Sci-Fi","Psychological"}, Status:"completed", TotalChapters:64, Description:"Alien parasites invade humanity by taking control of human bodies."},
        {ID:"ajin", Title:"Ajin: Demi-Human", Author:"Gamon Sakurai", Genres:[]string{"Action","Horror","Supernatural"}, Status:"completed", TotalChapters:86, Description:"Immortal beings are hunted and exploited by the government."},
        {ID:"erased", Title:"Erased", Author:"Kei Sanbe", Genres:[]string{"Mystery","Thriller","Supernatural"}, Status:"completed", TotalChapters:44, Description:"A man travels back to his childhood to prevent a series of murders."},
        {ID:"blue-lock", Title:"Blue Lock", Author:"Muneyuki Kaneshiro & Yusuke Nomura", Genres:[]string{"Sports","Drama","Psychological"}, Status:"ongoing", TotalChapters:300, Description:"Strikers compete in a ruthless program to create Japan’s ultimate soccer ace."},
        {ID:"ping-pong", Title:"Ping Pong", Author:"Taiyo Matsumoto", Genres:[]string{"Sports","Drama"}, Status:"completed", TotalChapters:55, Description:"A realistic and emotional take on competitive table tennis."},
        {ID:"fire-punch", Title:"Fire Punch", Author:"Tatsuki Fujimoto", Genres:[]string{"Dark Fantasy","Action","Psychological"}, Status:"completed", TotalChapters:83, Description:"A cursed man who burns eternally seeks meaning in a cruel frozen world."},
        {ID:"pluto", Title:"Pluto", Author:"Naoki Urasawa", Genres:[]string{"Sci-Fi","Mystery","Psychological"}, Status:"completed", TotalChapters:65, Description:"A robot detective investigates murders tied to the world’s strongest robots."},
        {ID:"gantz", Title:"Gantz", Author:"Hiroya Oku", Genres:[]string{"Action","Sci-Fi","Horror"}, Status:"completed", TotalChapters:383, Description:"Dead people are forced into deadly alien-hunting games."},
        {ID:"land-of-the-lustrous", Title:"Land of the Lustrous", Author:"Haruko Ichikawa", Genres:[]string{"Fantasy","Psychological","Action"}, Status:"completed", TotalChapters:108, Description:"Gem-based beings fight mysterious invaders while questioning identity and purpose."},
        {ID:"grand-blue", Title:"Grand Blue", Author:"Kenji Inoue & Kimitake Yoshioka", Genres:[]string{"Comedy","Slice of Life"}, Status:"ongoing", TotalChapters:100, Description:"College life chaos centered around diving, alcohol, and friendship."},
	}

	for _, m := range mangas {
		m.PreSave()
		_, err := DB.Exec(
			"INSERT OR IGNORE INTO manga (id, title, author, genres, status, total_chapters, description) VALUES (?, ?, ?, ?, ?, ?, ?)",
			m.ID, m.Title, m.Author, m.GenresString, m.Status, m.TotalChapters, m.Description,
		)
		if err != nil {
			return err
		}
	}
	log.Println("Seeded manga entries")
	return nil
}

// Close closes the database connection
func Close() error {
	if DB != nil {
		return DB.Close()
	}
	return nil
}

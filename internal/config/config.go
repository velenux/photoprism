package config

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/jinzhu/gorm"
	_ "github.com/jinzhu/gorm/dialects/mysql"
	_ "github.com/jinzhu/gorm/dialects/sqlite"
	gc "github.com/patrickmn/go-cache"
	"github.com/photoprism/photoprism/internal/entity"
	"github.com/photoprism/photoprism/internal/event"
	"github.com/photoprism/photoprism/internal/tidb"
	"github.com/photoprism/photoprism/internal/util"
	"github.com/sirupsen/logrus"
	tensorflow "github.com/tensorflow/tensorflow/tensorflow/go"
	"github.com/urfave/cli"
)

var log = event.Log

type Config struct {
	db     *gorm.DB
	cache  *gc.Cache
	config *Params
}

func initLogger(debug bool) {
	log.SetFormatter(&logrus.TextFormatter{
		DisableColors: false,
		FullTimestamp: true,
	})

	if debug {
		log.SetLevel(logrus.DebugLevel)
	} else {
		log.SetLevel(logrus.InfoLevel)
	}
}

func findExecutable(configBin, defaultBin string) (result string) {
	if configBin == "" {
		result = defaultBin
	} else {
		result = configBin
	}

	if path, err := exec.LookPath(result); err == nil {
		result = path
	}

	if !util.Exists(result) {
		result = ""
	}

	return result
}

func NewConfig(ctx *cli.Context) *Config {
	initLogger(ctx.GlobalBool("debug"))

	c := &Config{
		config: NewParams(ctx),
	}

	log.SetLevel(c.LogLevel())

	return c
}

// CreateDirectories creates all the folders that photoprism needs. These are:
// OriginalsPath
// ThumbnailsPath
// ImportPath
// ExportPath
func (c *Config) CreateDirectories() error {
	if err := os.MkdirAll(c.OriginalsPath(), os.ModePerm); err != nil {
		return err
	}

	if err := os.MkdirAll(c.ImportPath(), os.ModePerm); err != nil {
		return err
	}

	if err := os.MkdirAll(c.ExportPath(), os.ModePerm); err != nil {
		return err
	}

	if err := os.MkdirAll(c.ThumbnailsPath(), os.ModePerm); err != nil {
		return err
	}

	if err := os.MkdirAll(c.ResourcesPath(), os.ModePerm); err != nil {
		return err
	}

	if err := os.MkdirAll(c.SqlServerPath(), os.ModePerm); err != nil {
		return err
	}

	if err := os.MkdirAll(c.TensorFlowModelPath(), os.ModePerm); err != nil {
		return err
	}

	if err := os.MkdirAll(c.HttpStaticBuildPath(), os.ModePerm); err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(c.PIDFilename()), os.ModePerm); err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(c.LogFilename()), os.ModePerm); err != nil {
		return err
	}

	return nil
}

// connectToDatabase establishes a database connection.
// When used with the internal driver, it may create a new database server instance.
// It tries to do this 12 times with a 5 second sleep interval in between.
func (c *Config) connectToDatabase(ctx context.Context) error {
	dbDriver := c.DatabaseDriver()
	dbDsn := c.DatabaseDsn()

	if dbDriver == "" {
		return errors.New("can't connect: database driver not specified")
	}

	if dbDsn == "" {
		return errors.New("can't connect: database DSN not specified")
	}

	isTiDB := false
	initSuccess := false

	if dbDriver == DbTiDB {
		isTiDB = true
		dbDriver = DbMySQL
	}

	db, err := gorm.Open(dbDriver, dbDsn)
	if err != nil || db == nil {
		if isTiDB {
			log.Infof("starting database server at %s:%d\n", c.SqlServerHost(), c.SqlServerPort())

			go tidb.Start(ctx, c.SqlServerPath(), c.SqlServerPort(), c.SqlServerHost(), c.Debug())
		}

		for i := 1; i <= 12; i++ {
			time.Sleep(5 * time.Second)

			db, err = gorm.Open(dbDriver, dbDsn)

			if db != nil && err == nil {
				break
			}

			if isTiDB && !initSuccess {
				err = tidb.InitDatabase(c.SqlServerPort(), c.SqlServerPassword())

				if err != nil {
					log.Debug(err)
				} else {
					initSuccess = true
				}
			}
		}

		if err != nil || db == nil {
			log.Fatal(err)
		}
	}

	c.db = db
	return err
}

// Name returns the application name.
func (c *Config) Name() string {
	return c.config.Name
}

// Url returns the public server URL (default is "http://localhost:2342/").
func (c *Config) Url() string {
	if c.config.Url == "" {
		return "http://localhost:2342/"
	}

	return c.config.Url
}

// Title returns the site title (default is application name).
func (c *Config) Title() string {
	if c.config.Title == "" {
		return c.Name()
	}

	return c.config.Title
}

// Subtitle returns the site title.
func (c *Config) Subtitle() string {
	return c.config.Subtitle
}

// Description returns the site title.
func (c *Config) Description() string {
	return c.config.Description
}

// Author returns the site author / copyright.
func (c *Config) Author() string {
	return c.config.Author
}

// Description returns the twitter handle for sharing.
func (c *Config) Twitter() string {
	return c.config.Twitter
}

// Version returns the application version.
func (c *Config) Version() string {
	return c.config.Version
}

// TensorFlowVersion returns the TenorFlow framework version.
func (c *Config) TensorFlowVersion() string {
	return tensorflow.Version()
}

// Copyright returns the application copyright.
func (c *Config) Copyright() string {
	return c.config.Copyright
}

// Debug returns true if Debug mode is on.
func (c *Config) Debug() bool {
	return c.config.Debug
}

// Public returns true if app requires no authentication.
func (c *Config) Public() bool {
	return c.config.Public
}

// ReadOnly returns true if photo directories are write protected.
func (c *Config) ReadOnly() bool {
	return c.config.ReadOnly
}

// HideNSFW returns true if NSFW photos are hidden by default.
func (c *Config) HideNSFW() bool {
	return c.config.HideNSFW
}

// UploadNSFW returns true if NSFW photos can be uploaded.
func (c *Config) UploadNSFW() bool {
	return c.config.UploadNSFW
}

// AdminPassword returns the admin password.
func (c *Config) AdminPassword() string {
	if c.config.AdminPassword == "" {
		return "photoprism"
	}

	return c.config.AdminPassword
}

// LogLevel returns the logrus log level.
func (c *Config) LogLevel() logrus.Level {
	if c.Debug() {
		c.config.LogLevel = "debug"
	}

	if logLevel, err := logrus.ParseLevel(c.config.LogLevel); err == nil {
		return logLevel
	} else {
		return logrus.InfoLevel
	}
}

// ConfigFile returns the config file name.
func (c *Config) ConfigFile() string {
	return c.config.ConfigFile
}

// SettingsFile returns the user settings file name.
func (c *Config) SettingsFile() string {
	return c.ConfigPath() + "/settings.yml"
}

// ConfigPath returns the config path.
func (c *Config) ConfigPath() string {
	if c.config.ConfigPath == "" {
		return c.AssetsPath() + "/config"
	}

	return c.config.ConfigPath
}

// PIDFilename returns the filename for storing the server process id (pid).
func (c *Config) PIDFilename() string {
	if c.config.PIDFilename == "" {
		return c.AssetsPath() + "/photoprism.pid"
	}

	return c.config.PIDFilename
}

// LogFilename returns the filename for storing server logs.
func (c *Config) LogFilename() string {
	if c.config.LogFilename == "" {
		return c.AssetsPath() + "/photoprism.log"
	}

	return c.config.LogFilename
}

// DetachServer returns true if server should detach from console (daemon mode).
func (c *Config) DetachServer() bool {
	return c.config.DetachServer
}

// SqlServerHost returns the built-in SQL server host name or IP address (empty for all interfaces).
func (c *Config) SqlServerHost() string {
	if c.config.SqlServerHost == "" {
		return "127.0.0.1"
	}

	return c.config.SqlServerHost
}

// SqlServerPort returns the built-in SQL server port.
func (c *Config) SqlServerPort() uint {
	if c.config.SqlServerPort == 0 {
		return 4000
	}

	return c.config.SqlServerPort
}

// SqlServerPath returns the database storage path for TiDB.
func (c *Config) SqlServerPath() string {
	if c.config.SqlServerPath == "" {
		return c.ResourcesPath() + "/database"
	}

	return c.config.SqlServerPath
}

// SqlServerPassword returns the password for the built-in database server.
func (c *Config) SqlServerPassword() string {
	return c.config.SqlServerPassword
}

// HttpServerHost returns the built-in HTTP server host name or IP address (empty for all interfaces).
func (c *Config) HttpServerHost() string {
	if c.config.HttpServerHost == "" {
		return "0.0.0.0"
	}

	return c.config.HttpServerHost
}

// HttpServerPort returns the built-in HTTP server port.
func (c *Config) HttpServerPort() int {
	if c.config.HttpServerPort == 0 {
		return 2342
	}

	return c.config.HttpServerPort
}

// HttpServerMode returns the server mode.
func (c *Config) HttpServerMode() string {
	if c.config.HttpServerMode == "" {
		if c.Debug() {
			return "debug"
		}

		return "release"
	}

	return c.config.HttpServerMode
}

// HttpServerPassword returns the password for the user interface (optional).
func (c *Config) HttpServerPassword() string {
	return c.config.HttpServerPassword
}

// OriginalsPath returns the originals.
func (c *Config) OriginalsPath() string {
	return c.config.OriginalsPath
}

// ImportPath returns the import directory.
func (c *Config) ImportPath() string {
	return c.config.ImportPath
}

// ExportPath returns the export directory.
func (c *Config) ExportPath() string {
	return c.config.ExportPath
}

// SipsBin returns the sips binary file name.
func (c *Config) SipsBin() string {
	return findExecutable(c.config.SipsBin, "sips")
}

// DarktableBin returns the darktable-cli binary file name.
func (c *Config) DarktableBin() string {
	return findExecutable(c.config.DarktableBin, "darktable-cli")
}

// HeifConvertBin returns the heif-convert binary file name.
func (c *Config) HeifConvertBin() string {
	return findExecutable(c.config.HeifConvertBin, "heif-convert")
}

// ExifToolBin returns the exiftool binary file name.
func (c *Config) ExifToolBin() string {
	return findExecutable(c.config.ExifToolBin, "exiftool")
}

// DatabaseDriver returns the database driver name.
func (c *Config) DatabaseDriver() string {
	if c.config.DatabaseDriver == "" {
		return DbTiDB
	}

	return c.config.DatabaseDriver
}

// DatabaseDsn returns the database data source name (DSN).
func (c *Config) DatabaseDsn() string {
	if c.config.DatabaseDsn == "" {
		return "root:photoprism@tcp(localhost:4000)/photoprism?parseTime=true"
	}

	return c.config.DatabaseDsn
}

// CachePath returns the path to the cache.
func (c *Config) CachePath() string {
	return c.config.CachePath
}

// ThumbnailsPath returns the path to the cached thumbnails.
func (c *Config) ThumbnailsPath() string {
	return c.CachePath() + "/thumbnails"
}

// AssetsPath returns the path to the assets.
func (c *Config) AssetsPath() string {
	return c.config.AssetsPath
}

// ResourcesPath returns the path to the app resources like static files.
func (c *Config) ResourcesPath() string {
	if c.config.ResourcesPath == "" {
		return c.AssetsPath() + "/resources"
	}

	return c.config.ResourcesPath
}

// ExamplesPath returns the example files path.
func (c *Config) ExamplesPath() string {
	return c.ResourcesPath() + "/examples"
}

// TensorFlowModelPath returns the tensorflow model path.
func (c *Config) TensorFlowModelPath() string {
	return c.ResourcesPath() + "/nasnet"
}

// NSFWModelPath returns the NSFW tensorflow model path.
func (c *Config) NSFWModelPath() string {
	return c.ResourcesPath() + "/nsfw"
}

// HttpTemplatesPath returns the server templates path.
func (c *Config) HttpTemplatesPath() string {
	return c.ResourcesPath() + "/templates"
}

// HttpFaviconsPath returns the favicons path.
func (c *Config) HttpFaviconsPath() string {
	return c.HttpStaticPath() + "/favicons"
}

// HttpStaticPath returns the static server assets path (//server/static/*).
func (c *Config) HttpStaticPath() string {
	return c.ResourcesPath() + "/static"
}

// HttpStaticBuildPath returns the static build path (//server/static/build/*).
func (c *Config) HttpStaticBuildPath() string {
	return c.HttpStaticPath() + "/build"
}

// Cache returns the in-memory cache.
func (c *Config) Cache() *gc.Cache {
	if c.cache == nil {
		c.cache = gc.New(336*time.Hour, 30*time.Minute)
	}

	return c.cache
}

// Db returns the db connection.
func (c *Config) Db() *gorm.DB {
	if c.db == nil {
		log.Fatal("database not initialised.")
	}

	return c.db
}

// CloseDb closes the db connection (if any).
func (c *Config) CloseDb() error {
	if c.db != nil {
		if err := c.db.Close(); err == nil {
			c.db = nil
		} else {
			return err
		}
	}

	return nil
}

// MigrateDb will start a migration process.
func (c *Config) MigrateDb() {
	db := c.Db()

	// db.LogMode(true)

	db.AutoMigrate(
		&entity.File{},
		&entity.Photo{},
		&entity.Event{},
		&entity.Location{},
		&entity.Camera{},
		&entity.Lens{},
		&entity.Country{},
		&entity.Share{},

		&entity.Album{},
		&entity.PhotoAlbum{},
		&entity.Label{},
		&entity.Category{},
		&entity.PhotoLabel{},
		&entity.Keyword{},
		&entity.PhotoKeyword{},
	)
}

// ClientConfig returns a loaded and set configuration entity.
func (c *Config) ClientConfig() ClientConfig {
	db := c.Db()

	var cameras []*entity.Camera
	var albums []*entity.Album

	var position struct {
		PhotoUUID    string    `json:"uuid"`
		LocationID   string    `json:"olc"`
		PhotoLat     float64   `json:"lat"`
		PhotoLng     float64   `json:"lng"`
		TakenAt      time.Time `json:"utc"`
		TakenAtLocal time.Time `json:"time"`
	}

	db.Table("photos").
		Select("photo_uuid, location_id, photo_lat, photo_lng, taken_at, taken_at_local").
		Where("deleted_at IS NULL AND photo_lat != 0 AND photo_lng != 0").
		Order("taken_at DESC").
		Limit(1).Offset(0).
		Take(&position)

	var count = struct {
		Photos    uint `json:"photos"`
		Favorites uint `json:"favorites"`
		Private   uint `json:"private"`
		Stories   uint `json:"stories"`
		Labels    uint `json:"labels"`
		Albums    uint `json:"albums"`
		Countries uint `json:"countries"`
	}{}

	db.Table("photos").
		Select("COUNT(*) AS photos, SUM(photo_favorite) AS favorites, SUM(photo_private) AS private, SUM(photo_story) AS stories").
		Where("deleted_at IS NULL").
		Take(&count)

	db.Table("labels").
		Select("COUNT(*) AS labels").
		Where("(label_priority >= 0 || label_favorite = 1) && deleted_at IS NULL").
		Take(&count)

	db.Table("albums").
		Select("COUNT(*) AS albums").
		Where("deleted_at IS NULL").
		Take(&count)

	db.Table("countries").
		Select("COUNT(*) AS countries").
		Take(&count)

	type country struct {
		ID          string `json:"code"`
		CountryName string `json:"name"`
	}

	var countries []country

	db.Model(&entity.Country{}).
		Select("DISTINCT id, country_name").
		Scan(&countries)

	db.Where("deleted_at IS NULL").
		Limit(1000).Order("camera_model").
		Find(&cameras)

	db.Where("deleted_at IS NULL AND album_favorite = 1").
		Limit(20).Order("album_name").
		Find(&albums)

	jsHash := util.Hash(c.HttpStaticBuildPath() + "/app.js")
	cssHash := util.Hash(c.HttpStaticBuildPath() + "/app.css")

	result := ClientConfig{
		"name":        c.Name(),
		"url":         c.Url(),
		"title":       c.Title(),
		"subtitle":    c.Subtitle(),
		"description": c.Description(),
		"author":      c.Author(),
		"twitter":     c.Twitter(),
		"version":     c.Version(),
		"copyright":   c.Copyright(),
		"debug":       c.Debug(),
		"readonly":    c.ReadOnly(),
		"uploadNSFW":  c.UploadNSFW(),
		"public":      c.Public(),
		"albums":      albums,
		"cameras":     cameras,
		"countries":   countries,
		"thumbnails":  Thumbnails,
		"jsHash":      jsHash,
		"cssHash":     cssHash,
		"settings":    c.Settings(),
		"count":       count,
		"pos":         position,
	}

	return result
}

// Init initialises the Database.
func (c *Config) Init(ctx context.Context) error {
	return c.connectToDatabase(ctx)
}

// Shutdown closes open database connections.
func (c *Config) Shutdown() {
	if err := c.CloseDb(); err != nil {
		log.Errorf("could not close database connection: %s", err)
	} else {
		log.Info("closed database connection")
	}
}

// Settings returns the current user settings.
func (c *Config) Settings() *Settings {
	s := NewSettings()
	p := c.SettingsFile()

	if err := s.SetValuesFromFile(p); err != nil {
		log.Error(err)
	}

	return s
}

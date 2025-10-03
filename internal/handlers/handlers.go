package handlers

import (
	"fmt"
	"github.com/hashicorp/golang-lru/v2/expirable"
	echojwt "github.com/labstack/echo-jwt/v4"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"log"
	"net/http"
	"os"
	"recipes/internal/config"
	"recipes/internal/database"
	"recipes/internal/database/mongo"
	"recipes/internal/images_manager"
	"recipes/internal/jwt_manager"
	"recipes/internal/mail_sender"
	"recipes/internal/models"
	"recipes/internal/utils"
)

type Handlers struct {
	db     database.Database
	e      *echo.Echo
	jm     *jwt_manager.JwtManager
	jwt    *config.JwtConfig
	cfg    *config.RuntimeConfig
	ms     *mail_sender.MailSender
	imgLru *expirable.LRU[string, bool]
}

func New(cfg *config.Config) (*Handlers, error) {
	db, err := mongo.New(cfg.Db)
	if err != nil {
		return nil, err
	}

	jm := jwt_manager.JwtManager(*cfg.Jwt)

	// Create admin user
	if user, err := db.UserConflicts(models.UserDB{
		Username: cfg.Db.AdminUsername,
		Email:    cfg.Db.AdminMail,
	}); err != nil && !user.Admin {
		return nil, fmt.Errorf("username or email is already taken by a non-admin user")
	} else if err == nil {
		_, err = db.CreateUser(models.UserDB{
			Username: cfg.Db.AdminUsername,
			Email:    cfg.Db.AdminMail,
			Password: cfg.Db.AdminPassword,
			Admin:    true,
		})
		if err != nil {
			return nil, err
		}
	}

	imgLru := expirable.NewLRU[string, bool](0, func(filename string, hasTimeout bool) {
		if !hasTimeout {
			return
		}
		err := images_manager.Remove(cfg.Cfg.RecipeImageDir, filename)
		if err != nil {
			utils.LogError("Failed to remove recipe image", err)
		}
	}, cfg.Lru.RecipeImageTimeout)

	return &Handlers{
		db:     db,
		e:      echo.New(),
		jm:     &jm,
		jwt:    cfg.Jwt,
		cfg:    cfg.Cfg,
		ms:     mail_sender.New(cfg.Mails),
		imgLru: imgLru,
	}, nil
}

func (h *Handlers) RegisterEndpoints() {
	h.e.Use(middleware.CORS())
	h.e.Use(middleware.RecoverWithConfig(middleware.RecoverConfig{
		LogErrorFunc: func(c echo.Context, err error, stack []byte) error {
			_, _ = fmt.Fprintf(os.Stderr, "[PANIC RECOVERED] error: %v\nstack: %s\n", err, stack)
			return nil
		},
	}))

	adminMiddleware := func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			jwt := jwt_manager.GetJwt[*models.AccessJwt](c)
			if jwt.Admin {
				return next(c)
			}
			return errorResponse(http.StatusForbidden, "Forbidden", nil, c)
		}
	}

	unprotectedRouter := h.e.Group("/api/v1")
	protectedRouter := h.e.Group("/api/v1",
		h.QueryJwt,
		echojwt.WithConfig(echojwt.Config{
			NewClaimsFunc: jwt_manager.NewJwtClaims[models.AccessJwt],
			SigningKey:    []byte(h.jwt.AccessSecret),
			ContextKey:    "jwt",
		}),
	)

	unprotectedRouter.POST("/login", h.Login)
	unprotectedRouter.POST("/register", h.Register)
	unprotectedRouter.POST("/refresh", h.Refresh)
	unprotectedRouter.POST("/forgot-password", h.ForgotPassword)
	unprotectedRouter.POST("/forgot-password/:token", h.ResetPassword)

	unprotectedRouter.Static("/pictures", h.cfg.ImagesDir)

	// User self routes
	protectedRouter.GET("/users/me", h.GetMe)
	protectedRouter.PUT("/users/me", h.UpdateMe)
	protectedRouter.DELETE("/users/me", h.DeleteMe)
	protectedRouter.POST("/users/me/picture", h.UploadProfilePicture)
	protectedRouter.DELETE("/users/me/picture", h.DeleteProfilePicture)

	// User routes
	unprotectedRouter.GET("/users", h.GetUsers)
	unprotectedRouter.GET("/users/:id", h.GetUser)
	protectedRouter.PUT("/users/:id", h.UpdateUser, adminMiddleware)
	protectedRouter.DELETE("/users/:id", h.DeleteUser, adminMiddleware)
	protectedRouter.POST("/users/:id/picture", h.UpdateUserPicture, adminMiddleware)
	protectedRouter.DELETE("/users/:id/picture", h.DeleteUserPicture, adminMiddleware)

	// Recipes routes
	unprotectedRouter.GET("/recipes", h.GetRecipes)
	protectedRouter.POST("/recipes", h.CreateRecipe)
	unprotectedRouter.GET("/recipes/:id", h.GetRecipe)
	protectedRouter.PATCH("/recipes/:id", h.UpdateRecipe, h.RecipeAuthorMiddleware)
	protectedRouter.DELETE("/recipes/:id", h.DeleteRecipe, h.RecipeAuthorMiddleware)

	// Recipes images
	protectedRouter.POST("/recipe-pictures", h.SaveRecipeImage)
	protectedRouter.DELETE("/recipe-pictures/:id", h.DeleteRecipeImage)
	unprotectedRouter.Static("/recipe-pictures", h.cfg.RecipeImageDir)
}

func (h *Handlers) Run(port int) {
	log.Fatal(h.e.Start(fmt.Sprintf(":%d", port)))
}

// TODO: Limiter le nombre de caractères par titre de recette

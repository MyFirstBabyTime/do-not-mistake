package http

import (
	"github.com/MyFirstBabyTime/Server/domain"
	"github.com/gin-gonic/gin"
	"github.com/pkg/errors"
	"net/http"
)

// authHandler represent the http handler for article
type authHandler struct {
	aUsecase  domain.AuthUsecase
	validator validator
}

// validator is interface used for validating struct value
type validator interface {
	ValidateStruct(s interface{}) (err error)
}

// NewAuthHandler will initialize the auth/ resources endpoint
func NewAuthHandler(e *gin.Engine, au domain.AuthUsecase, v validator) {
	h := &authHandler{
		aUsecase:  au,
		validator: v,
	}

	e.POST("phones/phone-number/:phone_number/certify-code", h.SendCertifyCodeToPhone)
	//e.GET("/articles/:id", handler.GetByID)
	//e.DELETE("/articles/:id", handler.Delete)
}

// SendCertifyCodeToPhone is implement domain.AuthUsecase interface
func (ah *authHandler) SendCertifyCodeToPhone(c *gin.Context) {
	req := new(sendCertifyCodeToPhoneRequest)
	if err := c.BindUri(req); err != nil {
		resp := defaultResp(http.StatusUnprocessableEntity, 0, errors.Wrap(err, "failed to bind req").Error())
		c.JSON(http.StatusUnprocessableEntity, resp)
		return
	}

	if err := ah.validator.ValidateStruct(req); err != nil {
		resp := defaultResp(http.StatusBadRequest, 0, errors.Wrap(err, "invalid request").Error())
		c.JSON(http.StatusBadRequest, resp)
		return
	}

	switch err := ah.aUsecase.SendCertifyCodeToPhone(c.Request.Context(), req.PhoneNumber); tErr := err.(type) {
	case nil:
		resp := defaultResp(http.StatusOK, 0, "succeed to send certify code to phone")
		c.JSON(http.StatusOK, resp)
	case usecaseErr:
		resp := defaultResp(tErr.Status(), tErr.Code(), tErr.Error())
		c.JSON(tErr.Status(), resp)
	default:
		msg := errors.Wrap(err, "SendCertifyCodeToPhone return unexpected error").Error()
		resp := defaultResp(http.StatusInternalServerError, 0, msg)
		c.JSON(http.StatusInternalServerError, resp)
	}
	return
}

func defaultResp(status, code int, msg string) interface{} {
	return struct {
		Status  int    `json:"status"`
		Code    int    `json:"code"`
		Message string `json:"message"`
	}{status, code, msg}
}

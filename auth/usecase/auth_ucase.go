package usecase

import (
	"context"
	"database/sql"
	"fmt"
	"github.com/pkg/errors"
	"time"

	"github.com/MyFirstBabyTime/Server/domain"
	"github.com/MyFirstBabyTime/Server/tx"
)

// authUsecase is used for usecase layer which implement domain.AuthUsecase interface
type authUsecase struct {
	// parentAuthRepository is repository interface about domain.ParentAuth model
	parentAuthRepository domain.ParentAuthRepository

	// parentPhoneCertifyRepository is repository interface about domain.ParentPhoneCertify model
	parentPhoneCertifyRepository domain.ParentPhoneCertifyRepository

	// txHandler is used for handling transaction to begin & commit or rollback
	txHandler txHandler

	// messageAgency is used as agency about message API
	messageAgency messageAgency

	// messageAgency is used as handler about hashing
	hashHandler hashHandler

	// jwtHandler is used as handler about jwt
	jwtHandler jwtHandler
}

// AuthUsecase return implementation of domain.AuthUsecase
func AuthUsecase(
	par domain.ParentAuthRepository,
	ppr domain.ParentPhoneCertifyRepository,
	th txHandler,
	ma messageAgency,
	hh hashHandler,
	jh jwtHandler,
) domain.AuthUsecase {
	return &authUsecase{
		parentAuthRepository:         par,
		parentPhoneCertifyRepository: ppr,

		txHandler:     th,
		messageAgency: ma,
		hashHandler:   hh,
		jwtHandler:    jh,
	}
}

// txHandler is used for handling transaction to begin & commit or rollback
type txHandler interface {
	// BeginTx method start transaction (get option from ctx)
	BeginTx(ctx context.Context, opts interface{}) (tx tx.Context, err error)

	// Commit method commit transaction
	Commit(tx tx.Context) (err error)

	// Rollback method rollback transaction
	Rollback(tx tx.Context) (err error)
}

// messageAgency is agency that agent various API about message
type messageAgency interface {
	// SendSMSToOne method send SMS message to one receiver
	SendSMSToOne(receiver, content string) (err error)
}

// hashHandler is interface about hash handler
type hashHandler interface {
	// GenerateHashWithMinSalt generate & return hashed value from password with minimum salt
	GenerateHashWithMinSalt(pw string) (hash string, err error)

	// CompareHashAndPW compare hashed value and password & return error
	CompareHashAndPW(hash, pw string) (err error)
}

// jwtHandler is interface about JWT handler
type jwtHandler interface {
	// GenerateUUIDJWT generate & return JWT UUID token with type & time
	GenerateUUIDJWT(uuid, _type string, time time.Duration) (token string, err error)
}

// SendCertifyCodeToPhone is implement domain.AuthUsecase interface
func (au *authUsecase) SendCertifyCodeToPhone(ctx context.Context, pn string) (err error) {
	_tx, err := au.txHandler.BeginTx(ctx, nil)
	if err != nil {
		err = errors.Wrap(err, "failed to begin transaction")
		return
	}

	ppc, err := au.parentPhoneCertifyRepository.GetByPhoneNumber(_tx, pn)
	switch err.(type) {
	case nil:
		if ppc.ParentUUID.Valid {
			err = conflictErr{errors.New("this phone number is already in use"), phoneAlreadyInUse}
			_ = au.txHandler.Rollback(_tx)
			return
		}
		ppc.CertifyCode = ppc.GenerateCertifyCode()
		ppc.Certified = sql.NullBool{Bool: false, Valid: true}
		switch err = au.parentPhoneCertifyRepository.Update(_tx, &ppc); err.(type) {
		case nil:
			break
		default:
			err = internalServerErr{errors.Wrap(err, "phone Update return unexpected error")}
			_ = au.txHandler.Rollback(_tx)
			return
		}
	case rowNotExistErr:
		ppc = domain.ParentPhoneCertify{PhoneNumber: pn}
		ppc.CertifyCode = ppc.GenerateCertifyCode()
		switch err = au.parentPhoneCertifyRepository.Store(_tx, &ppc); err.(type) {
		case nil:
			break
		default:
			err = internalServerErr{errors.Wrap(err, "phone Store return unexpected error")}
			_ = au.txHandler.Rollback(_tx)
			return
		}
	default:
		err = internalServerErr{errors.Wrap(err, "GetByPhoneNumber return unexpected error")}
		_ = au.txHandler.Rollback(_tx)
		return
	}

	content := fmt.Sprintf("[육아는 처음이지 인증 번호]\n회원가입 인증 번호: %d", ppc.CertifyCode)
	if err = au.messageAgency.SendSMSToOne(ppc.PhoneNumber, content); err != nil {
		err = internalServerErr{errors.Wrap(err, "SendSMSToOne return unexpected error")}
		_ = au.txHandler.Rollback(_tx)
		return
	}

	_ = au.txHandler.Commit(_tx)
	return nil
}

// CertifyPhoneWithCode is implement domain.AuthUsecase interface
func (au *authUsecase) CertifyPhoneWithCode(ctx context.Context, pn string, code int64) (err error) {
	_tx, err := au.txHandler.BeginTx(ctx, nil)
	if err != nil {
		err = errors.Wrap(err, "failed to begin transaction")
		return
	}

	ppc, err := au.parentPhoneCertifyRepository.GetByPhoneNumber(_tx, pn)
	switch err.(type) {
	case nil:
		if ppc.Certified.Valid && ppc.Certified.Bool {
			err = conflictErr{errors.New("this phone number is already certified"), phoneAlreadyCertified}
			_ = au.txHandler.Rollback(_tx)
			return
		}
		if code != ppc.CertifyCode {
			err = conflictErr{errors.New("incorrect certify code to that phone number"), incorrectCertifyCode}
			_ = au.txHandler.Rollback(_tx)
			return
		}
		ppc.Certified = sql.NullBool{Bool: true, Valid: true}
		switch err = au.parentPhoneCertifyRepository.Update(_tx, &ppc); err.(type) {
		case nil:
			break
		default:
			err = internalServerErr{errors.Wrap(err, "phone Update return unexpected error")}
			_ = au.txHandler.Rollback(_tx)
			return
		}
	case rowNotExistErr:
		err = notFoundErr{errors.New("not exist phone number")}
		_ = au.txHandler.Rollback(_tx)
		return
	default:
		err = internalServerErr{errors.Wrap(err, "GetByPhoneNumber return unexpected error")}
		_ = au.txHandler.Rollback(_tx)
		return
	}

	_ = au.txHandler.Commit(_tx)
	return nil
}

// SignUpParent is implement domain.AuthUsecase interface
func (au *authUsecase) SignUpParent(ctx context.Context, pa *domain.ParentAuth, pn string) (err error) {
	_tx, err := au.txHandler.BeginTx(ctx, nil)
	if err != nil {
		err = errors.Wrap(err, "failed to begin transaction")
		return
	}

	ppc, err := au.parentPhoneCertifyRepository.GetByPhoneNumber(_tx, pn)
	if err == nil && ppc.Certified.Valid && ppc.Certified.Bool {
		if pa.PW, err = au.hashHandler.GenerateHashWithMinSalt(pa.PW); err != nil {
			err = internalServerErr{errors.Wrap(err, "failed to GenerateHashWithMinSalt")}
			_ = au.txHandler.Rollback(_tx)
			return
		}
		if pa.UUID, err = au.parentAuthRepository.GetAvailableUUID(_tx); err != nil {
			pa.UUID = pa.GenerateRandomUUID()
		}
		switch err = au.parentAuthRepository.Store(_tx, pa); tErr := err.(type) {
		case nil:
			break
		case invalidModelErr:
			err = internalServerErr{errors.Wrap(err, "parent auth Store return invalid model")}
			_ = au.txHandler.Rollback(_tx)
			return
		case entryDuplicateErr:
			switch tErr.DuplicateKey() {
			case "id":
				err = conflictErr{errors.New("this parent ID is already in use"), parentIDAlreadyInUse}
				_ = au.txHandler.Rollback(_tx)
				return
			default:
				err = internalServerErr{errors.Wrap(err, "parent auth Store return unexpected duplicate error")}
				_ = au.txHandler.Rollback(_tx)
				return
			}
		default:
			err = internalServerErr{errors.Wrap(err, "parent auth Store return unexpected error")}
			_ = au.txHandler.Rollback(_tx)
			return
		}
	} else {
		if _, ok := err.(rowNotExistErr); err == nil || ok {
			err = conflictErr{errors.New("this phone number is not certified"), uncertifiedPhone}
			_ = au.txHandler.Rollback(_tx)
			return
		} else {
			err = internalServerErr{errors.Wrap(err, "GetByPhoneNumber return unexpected error")}
			_ = au.txHandler.Rollback(_tx)
			return
		}
	}

	_ = au.txHandler.Commit(_tx)
	return nil
}

// LoginParentAuth is implement domain.AuthUsecase interface
func (au *authUsecase) LoginParentAuth(ctx context.Context, id, pw string) (uuid, token string, err error) {
	_tx, err := au.txHandler.BeginTx(ctx, nil)
	if err != nil {
		err = errors.Wrap(err, "failed to begin transaction")
		return
	}

	pa, err := au.parentAuthRepository.GetByID(_tx, id)
	switch err.(type) {
	case nil:
		switch err = au.hashHandler.CompareHashAndPW(pa.PW, pw); err.(type) {
		case nil:
			break
		case interface{ Mismatch() }:
			err = conflictErr{errors.New("incorrect password"), incorrectParentPW}
			_ = au.txHandler.Rollback(_tx)
			return
		default:
			err = internalServerErr{errors.Wrap(err, "CompareHashAndPW return unexpected error")}
			_ = au.txHandler.Rollback(_tx)
			return
		}
	case rowNotExistErr:
		err = conflictErr{errors.New("not exist parent ID"), notExistParentID}
		_ = au.txHandler.Rollback(_tx)
		return
	default:
		err = internalServerErr{errors.Wrap(err, "GetByID return unexpected error")}
		_ = au.txHandler.Rollback(_tx)
		return
	}

	uuid = pa.UUID
	token, err = au.jwtHandler.GenerateUUIDJWT(pa.UUID, "access_token", time.Hour*24)
	err = nil

	_ = au.txHandler.Commit(_tx)
	return
}

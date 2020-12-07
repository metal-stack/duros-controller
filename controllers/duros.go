package controllers

import (
	"context"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	durosv2 "github.com/metal-stack/duros-go/api/duros/v2"
)

// createProjectIfNotExist check for duros project and create if required
func (r *DurosReconciler) createProjectIfNotExist(ctx context.Context, projectID string) (*durosv2.Project, error) {

	p, err := r.DurosClient.GetProject(ctx, &durosv2.GetProjectRequest{Name: projectID}, nil)
	if err != nil {
		s, ok := status.FromError(err)
		if !ok {
			return nil, fmt.Errorf("unable to parse duros error")
		}
		switch s.Code() {
		case codes.NotFound:
			p, err := r.DurosClient.CreateProject(ctx, &durosv2.CreateProjectRequest{Name: projectID}, nil)
			if err != nil {
				return p, err
			}
		default:
			return nil, err
		}
	}
	return p, nil
}

func (r *DurosReconciler) createProjectCredentialsIfNotExist(ctx context.Context, projectID string, adminKey []byte) (*durosv2.Credential, error) {
	id := projectID + ":root"
	cred, err := r.DurosClient.GetCredential(ctx, &durosv2.GetCredentialRequest{ID: id, ProjectName: projectID})
	if err != nil {
		s, ok := status.FromError(err)
		if !ok {
			return nil, fmt.Errorf("unable to parse duros error")
		}
		switch s.Code() {
		case codes.NotFound:
			// create credential
			key, err := extract(adminKey)
			if err != nil {
				return nil, err
			}
			pubkeyBytes, err := publicKeyToBytes(&key.PublicKey)
			if err != nil {
				return nil, fmt.Errorf("unable to convert adminKey public key into pem encoded byte slice:%v", err)
			}
			// Regarding  CreateCredentialRequest.Payload:
			// there are 3 entities at play here:
			// private key: used to sign JWTs. stays on your (or your customers') laptop forever.
			// public key (related to the private key above): uploaded to LightOS as credential, used to authorize JWTs.
			// typically you'll upload one of those per project on behalf of your tenants.
			// JWTs: signed using the private key above offline (wherever or whenever you want),
			// validated on LightOS at API CreateCredentialRequestrequest time using the public key (aka "credential" for the purposes of this discussion).
			// so, you can have as many JWTs as you wish, but as per our previous discussions,
			// you'll probably generate one priv/pub key pair per project (tenant), upload its pub key as cred into the project on LightOS,
			// sign a single JWT using the priv key and deploy it into your customer's K8s cluster as a secret.
			// so the Payload above is the PEM-encoded public key, not the JWT.
			// you'll get a much more sensible docs bundle with the release, of course, this is just a preview.
			cred, err = r.DurosClient.CreateCredential(ctx, &durosv2.CreateCredentialRequest{
				ProjectName: projectID,
				ID:          id,
				Type:        durosv2.CredsType_RS256PubKey,
				Payload:     pubkeyBytes,
			})
			if err != nil {
				return nil, err
			}
			return cred, nil
		default:
			return nil, err
		}
	}
	return cred, nil
}

func extract(adminKey []byte) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode(adminKey)
	if block == nil {
		return nil, fmt.Errorf("unable to decode the admin key into pem format")
	}
	key, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse RSA key pair: %s", err)
	}
	return key, nil
}

// publicKeyToBytes public key to bytes
func publicKeyToBytes(pub *rsa.PublicKey) ([]byte, error) {
	pubASN1, err := x509.MarshalPKIXPublicKey(pub)
	if err != nil {
		return nil, err
	}

	pubBytes := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PUBLIC KEY",
		Bytes: pubASN1,
	})

	return pubBytes, nil
}

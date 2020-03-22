package s3

import (
	"crypto/rand"
	"crypto/sha256"
	"fmt"
	"golang.org/x/crypto/hkdf"
	"io"
	"path"
	"time"

	"github.com/minio/minio-go/v6"
	"github.com/minio/sio"
)

type S3AO struct {
	client        *minio.Client
	bucket        string
	encryptionKey string
}

// Initialize S3AO
func Init(endpoint, bucket, region, accessKey, secretKey, encryptionKey string) (S3AO, error) {
	var s3ao S3AO
	ssl := false

	// Set up client for S3AO
	minioClient, err := minio.New(endpoint, accessKey, secretKey, ssl)
	if err != nil {
		return s3ao, err
	}
	s3ao.client = minioClient
	s3ao.bucket = bucket
	s3ao.encryptionKey = encryptionKey

	fmt.Printf("Established session to S3AO at %s\n", endpoint)

	// Ensure that the bucket exists
	found, err := s3ao.client.BucketExists(bucket)
	if err != nil {
		fmt.Printf("Unable to check if S3AO bucket exists: %s\n", err.Error())
		return s3ao, err
	}
	if found {
		fmt.Printf("Found S3AO bucket: %s\n", bucket)
	} else {
		t0 := time.Now()
		if err := s3ao.client.MakeBucket(bucket, region); err != nil {
			fmt.Printf("%s\n", err.Error())
		}
		fmt.Printf("Created S3AO bucket: %s in %.3fs\n", bucket, time.Since(t0).Seconds())
	}
	return s3ao, nil
}

func (s S3AO) GenerateNonce() []byte {
	var nonce []byte
	if _, err := io.ReadFull(rand.Reader, nonce[:]); err != nil {
		fmt.Printf("Failed to read random data: %v", err) // add error handling
	}
	return nonce
}

func (s S3AO) PutObject(bin string, filename string, data io.Reader, size int64) ([]byte, error) {
	t0 := time.Now()

	// Hash the path in S3
	b := sha256.New()
	b.Write([]byte(bin))
	f := sha256.New()
	f.Write([]byte(filename))
	objectKey := path.Join(fmt.Sprintf("%x", b.Sum(nil)), fmt.Sprintf("%x", f.Sum(nil)))

	// the master key used to derive encryption keys
	// this key must be keep secret
	//masterkey, err := hex.DecodeString(s.encryptionKey) // use your own key here
	//if err != nil {
	//	fmt.Printf("Cannot decode hex key: %v", err) // add error handling
	//	return err
	//}
	masterkey := []byte(s.encryptionKey)

	// generate a random nonce to derive an encryption key from the master key
	// this nonce must be saved to be able to decrypt the data again - it is not
	// required to keep it secret
	nonce := s.GenerateNonce()

	// derive an encryption key from the master key and the nonce
	var key [32]byte
	kdf := hkdf.New(sha256.New, masterkey, nonce[:], nil)
	if _, err := io.ReadFull(kdf, key[:]); err != nil {
		fmt.Printf("Failed to derive encryption key: %v", err) // add error handling
		return nonce, err
	}

	encrypted, err := sio.EncryptReader(data, sio.Config{Key: key[:]})
	if err != nil {
		fmt.Printf("Failed to encrypted reader: %v", err) // add error handling
		return nonce, err
	}

	encryptedSize, err := sio.EncryptedSize(uint64(size))
	if err != nil {
		fmt.Printf("Failed to compute size of encrypted object: %v", err) // add error handling
		return nonce, err
	}

	n, err := s.client.PutObject(s.bucket, objectKey, encrypted, int64(encryptedSize), minio.PutObjectOptions{ContentType: "application/octet-stream"})
	if err != nil {
		fmt.Printf("Unable to put object: %s\n", err.Error())
	}
	fmt.Printf("Uploaded object: %s (%d bytes) in %.3fs\n", objectKey, n, time.Since(t0).Seconds())
	return nonce, nil
}

func (s S3AO) RemoveObject(bin string, filename string) error {
	key := path.Join(bin, filename)
	err := s.RemoveKey(key)
	return err
}

func (s S3AO) RemoveKey(key string) error {
	t0 := time.Now()
	err := s.client.RemoveObject(s.bucket, key)
	if err != nil {
		fmt.Printf("Unable to remove object: %s\n", err.Error())
	}
	fmt.Printf("Removed object: %s in %.3fs\n", key, time.Since(t0).Seconds())
	return nil
}

func (s S3AO) listObjects() (objects []string, err error) {
	// Create a done channel to control 'ListObjects' go routine.
	doneCh := make(chan struct{})

	// Indicate to our routine to exit cleanly upon return.
	defer close(doneCh)

	isRecursive := true
	objectCh := s.client.ListObjectsV2(s.bucket, "", isRecursive, doneCh)
	for object := range objectCh {
		if object.Err != nil {
			return objects, object.Err
		}
		objects = append(objects, object.Key)
	}
	return objects, nil
}

func (s S3AO) RemoveBucket() error {
	t0 := time.Now()
	objects, err := s.listObjects()
	if err != nil {
		fmt.Printf("Unable to list objects: %s\n", err.Error())
	}

	// ReoveObject on all objects
	for _, object := range objects {
		if err := s.RemoveKey(object); err != nil {
			return err
		}
	}

	// RemoveBucket
	if err := s.client.RemoveBucket(s.bucket); err != nil {
		return err
	}

	fmt.Printf("Removed bucket in %.3fs\n", time.Since(t0).Seconds())
	return nil
}

func (s S3AO) GetObject(bin string, filename string, nonce []byte) (io.Reader, error) {

	// Hash the path in S3
	b := sha256.New()
	b.Write([]byte(bin))
	f := sha256.New()
	f.Write([]byte(filename))
	objectKey := path.Join(fmt.Sprintf("%x", b.Sum(nil)), fmt.Sprintf("%x", f.Sum(nil)))
	var object io.Reader

	// the master key used to derive encryption keys
	//masterkey, err := hex.DecodeString(s.encryptionKey) // use your own key here
	//if err != nil {
	//	fmt.Printf("Cannot decode hex key: %v", err) // add error handling
	//	return object, err
	//}
	masterkey := []byte(s.encryptionKey)

	// derive the encryption key from the master key and the nonce
	var key [32]byte
	kdf := hkdf.New(sha256.New, masterkey, nonce[:], nil)
	if _, err := io.ReadFull(kdf, key[:]); err != nil {
		fmt.Printf("Failed to derive encryption key: %v", err) // add error handling
		return object, err
	}

	object, err := s.client.GetObject(s.bucket, objectKey, minio.GetObjectOptions{})
	if err != nil {
		return object, err
	}

	decryptedObject, err := sio.DecryptReader(object, sio.Config{Key: key[:]})
	if err != nil {
		if _, ok := err.(sio.Error); ok {
			fmt.Printf("Malformed encrypted data: %v", err) // add error handling - here we know that the data is malformed/not authentic.
			return object, err
		}
		fmt.Printf("Failed to decrypt data: %v", err) // add error handling
		return object, err
	}
	return decryptedObject, err
}

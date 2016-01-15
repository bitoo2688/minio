/*
 * Minio Cloud Storage, (C) 2015 Minio, Inc.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package main

import (
	"encoding/hex"
	"io/ioutil"
	"net/http"

	"github.com/gorilla/mux"
	"github.com/minio/minio-xl/pkg/crypto/sha256"
	"github.com/minio/minio-xl/pkg/probe"
	"github.com/minio/minio/pkg/fs"
)

// ListMultipartUploadsHandler - GET Bucket (List Multipart uploads)
// -------------------------
// This operation lists in-progress multipart uploads. An in-progress
// multipart upload is a multipart upload that has been initiated,
// using the Initiate Multipart Upload request, but has not yet been
// completed or aborted. This operation returns at most 1,000 multipart
// uploads in the response.
//
func (api CloudStorageAPI) ListMultipartUploadsHandler(w http.ResponseWriter, req *http.Request) {
	vars := mux.Vars(req)
	bucket := vars["bucket"]

	if !api.Anonymous {
		if isRequestRequiresACLCheck(req) {
			if api.Filesystem.IsPrivateBucket(bucket) {
				writeErrorResponse(w, req, AccessDenied, req.URL.Path)
				return
			}
		}
	}

	resources := getBucketMultipartResources(req.URL.Query())
	if resources.MaxUploads < 0 {
		writeErrorResponse(w, req, InvalidMaxUploads, req.URL.Path)
		return
	}
	if resources.MaxUploads == 0 {
		resources.MaxUploads = maxObjectList
	}

	resources, err := api.Filesystem.ListMultipartUploads(bucket, resources)
	if err != nil {
		errorIf(err.Trace(), "ListMultipartUploads failed.", nil)
		switch err.ToGoError().(type) {
		case fs.BucketNotFound:
			writeErrorResponse(w, req, NoSuchBucket, req.URL.Path)
		default:
			writeErrorResponse(w, req, InternalError, req.URL.Path)
		}
		return
	}
	// generate response
	response := generateListMultipartUploadsResponse(bucket, resources)
	encodedSuccessResponse := encodeSuccessResponse(response)
	// write headers
	setCommonHeaders(w, len(encodedSuccessResponse))
	// write body
	w.Write(encodedSuccessResponse)
}

// ListObjectsHandler - GET Bucket (List Objects)
// -------------------------
// This implementation of the GET operation returns some or all (up to 1000)
// of the objects in a bucket. You can use the request parameters as selection
// criteria to return a subset of the objects in a bucket.
//
func (api CloudStorageAPI) ListObjectsHandler(w http.ResponseWriter, req *http.Request) {
	vars := mux.Vars(req)
	bucket := vars["bucket"]

	if !api.Anonymous {
		if isRequestRequiresACLCheck(req) {
			if api.Filesystem.IsPrivateBucket(bucket) {
				writeErrorResponse(w, req, AccessDenied, req.URL.Path)
				return
			}
		}
	}
	resources := getBucketResources(req.URL.Query())
	if resources.Maxkeys < 0 {
		writeErrorResponse(w, req, InvalidMaxKeys, req.URL.Path)
		return
	}
	if resources.Maxkeys == 0 {
		resources.Maxkeys = maxObjectList
	}

	listReq := fs.ListObjectsReq{
		Prefix:    resources.Prefix,
		Marker:    resources.Marker,
		Delimiter: resources.Delimiter,
		MaxKeys:   resources.Maxkeys,
	}
	// objects, resources, err := api.Filesystem.ListObjects(bucket, req)
	listResp, err := api.Filesystem.ListObjects(bucket, listReq)
	if err == nil {
		// generate response
		// response := generateListObjectsResponse(bucket, objects, resources)
		response := generateListObjectsResponse(bucket, listReq, listResp)
		encodedSuccessResponse := encodeSuccessResponse(response)
		// write headers
		setCommonHeaders(w, len(encodedSuccessResponse))
		// write body
		w.Write(encodedSuccessResponse)
		return
	}
	switch err.ToGoError().(type) {
	case fs.BucketNameInvalid:
		writeErrorResponse(w, req, InvalidBucketName, req.URL.Path)
	case fs.BucketNotFound:
		writeErrorResponse(w, req, NoSuchBucket, req.URL.Path)
	case fs.ObjectNotFound:
		writeErrorResponse(w, req, NoSuchKey, req.URL.Path)
	case fs.ObjectNameInvalid:
		writeErrorResponse(w, req, NoSuchKey, req.URL.Path)
	default:
		errorIf(err.Trace(), "ListObjects failed.", nil)
		writeErrorResponse(w, req, InternalError, req.URL.Path)
	}
}

// ListBucketsHandler - GET Service
// -----------
// This implementation of the GET operation returns a list of all buckets
// owned by the authenticated sender of the request.
func (api CloudStorageAPI) ListBucketsHandler(w http.ResponseWriter, req *http.Request) {
	if !api.Anonymous {
		if isRequestRequiresACLCheck(req) {
			writeErrorResponse(w, req, AccessDenied, req.URL.Path)
			return
		}
	}
	buckets, err := api.Filesystem.ListBuckets()
	if err == nil {
		// generate response
		response := generateListBucketsResponse(buckets)
		encodedSuccessResponse := encodeSuccessResponse(response)
		// write headers
		setCommonHeaders(w, len(encodedSuccessResponse))
		// write response
		w.Write(encodedSuccessResponse)
		return
	}
	errorIf(err.Trace(), "ListBuckets failed.", nil)
	writeErrorResponse(w, req, InternalError, req.URL.Path)
}

// PutBucketHandler - PUT Bucket
// ----------
// This implementation of the PUT operation creates a new bucket for authenticated request
func (api CloudStorageAPI) PutBucketHandler(w http.ResponseWriter, req *http.Request) {
	vars := mux.Vars(req)
	bucket := vars["bucket"]

	if !api.Anonymous {
		if isRequestRequiresACLCheck(req) {
			writeErrorResponse(w, req, AccessDenied, req.URL.Path)
			return
		}
	}

	// read from 'x-amz-acl'
	aclType := getACLType(req)
	if aclType == unsupportedACLType {
		writeErrorResponse(w, req, NotImplemented, req.URL.Path)
		return
	}

	var signature *fs.Signature
	if !api.Anonymous {
		// Init signature V4 verification
		if isRequestSignatureV4(req) {
			var err *probe.Error
			signature, err = initSignatureV4(req)
			if err != nil {
				errorIf(err.Trace(), "Initializing signature v4 failed.", nil)
				writeErrorResponse(w, req, InternalError, req.URL.Path)
				return
			}
		}
	}

	// if body of request is non-nil then check for validity of Content-Length
	if req.Body != nil {
		/// if Content-Length missing, deny the request
		if req.Header.Get("Content-Length") == "" {
			writeErrorResponse(w, req, MissingContentLength, req.URL.Path)
			return
		}
		if signature != nil {
			locationBytes, err := ioutil.ReadAll(req.Body)
			if err != nil {
				sh := sha256.New()
				sh.Write(locationBytes)
				ok, perr := signature.DoesSignatureMatch(hex.EncodeToString(sh.Sum(nil)))
				if perr != nil {
					errorIf(perr.Trace(), "MakeBucket failed.", nil)
					writeErrorResponse(w, req, InternalError, req.URL.Path)
					return
				}
				if !ok {
					writeErrorResponse(w, req, SignatureDoesNotMatch, req.URL.Path)
					return
				}
			}
		}
	}

	err := api.Filesystem.MakeBucket(bucket, getACLTypeString(aclType))
	if err != nil {
		errorIf(err.Trace(), "MakeBucket failed.", nil)
		switch err.ToGoError().(type) {
		case fs.BucketNameInvalid:
			writeErrorResponse(w, req, InvalidBucketName, req.URL.Path)
		case fs.BucketExists:
			writeErrorResponse(w, req, BucketAlreadyExists, req.URL.Path)
		default:
			writeErrorResponse(w, req, InternalError, req.URL.Path)
		}
		return
	}
	// Make sure to add Location information here only for bucket
	w.Header().Set("Location", "/"+bucket)
	writeSuccessResponse(w)
}

// PostPolicyBucketHandler - POST policy
// ----------
// This implementation of the POST operation handles object creation with a specified
// signature policy in multipart/form-data
func (api CloudStorageAPI) PostPolicyBucketHandler(w http.ResponseWriter, req *http.Request) {
	// if body of request is non-nil then check for validity of Content-Length
	if req.Body != nil {
		/// if Content-Length missing, deny the request
		size := req.Header.Get("Content-Length")
		if size == "" {
			writeErrorResponse(w, req, MissingContentLength, req.URL.Path)
			return
		}
	}

	// Here the parameter is the size of the form data that should
	// be loaded in memory, the remaining being put in temporary
	// files
	reader, err := req.MultipartReader()
	if err != nil {
		errorIf(probe.NewError(err), "Unable to initialize multipart reader.", nil)
		writeErrorResponse(w, req, MalformedPOSTRequest, req.URL.Path)
		return
	}

	fileBody, formValues, perr := extractHTTPFormValues(reader)
	if perr != nil {
		errorIf(perr.Trace(), "Unable to parse form values.", nil)
		writeErrorResponse(w, req, MalformedPOSTRequest, req.URL.Path)
		return
	}
	bucket := mux.Vars(req)["bucket"]
	formValues["Bucket"] = bucket
	object := formValues["Key"]
	signature, perr := initPostPresignedPolicyV4(formValues)
	if perr != nil {
		errorIf(perr.Trace(), "Unable to initialize post policy presigned.", nil)
		writeErrorResponse(w, req, MalformedPOSTRequest, req.URL.Path)
		return
	}
	var ok bool
	if ok, perr = signature.DoesPolicySignatureMatch(formValues["X-Amz-Date"]); perr != nil {
		errorIf(perr.Trace(), "Unable to verify signature.", nil)
		writeErrorResponse(w, req, SignatureDoesNotMatch, req.URL.Path)
		return
	}
	if ok == false {
		writeErrorResponse(w, req, SignatureDoesNotMatch, req.URL.Path)
		return
	}
	if perr = applyPolicy(formValues); perr != nil {
		errorIf(perr.Trace(), "Invalid request, policy doesn't match with the endpoint.", nil)
		writeErrorResponse(w, req, MalformedPOSTRequest, req.URL.Path)
		return
	}
	metadata, perr := api.Filesystem.CreateObject(bucket, object, "", 0, fileBody, nil)
	if perr != nil {
		errorIf(perr.Trace(), "CreateObject failed.", nil)
		switch perr.ToGoError().(type) {
		case fs.RootPathFull:
			writeErrorResponse(w, req, RootPathFull, req.URL.Path)
		case fs.BucketNotFound:
			writeErrorResponse(w, req, NoSuchBucket, req.URL.Path)
		case fs.BucketNameInvalid:
			writeErrorResponse(w, req, InvalidBucketName, req.URL.Path)
		case fs.BadDigest:
			writeErrorResponse(w, req, BadDigest, req.URL.Path)
		case fs.SignatureDoesNotMatch:
			writeErrorResponse(w, req, SignatureDoesNotMatch, req.URL.Path)
		case fs.IncompleteBody:
			writeErrorResponse(w, req, IncompleteBody, req.URL.Path)
		case fs.EntityTooLarge:
			writeErrorResponse(w, req, EntityTooLarge, req.URL.Path)
		case fs.InvalidDigest:
			writeErrorResponse(w, req, InvalidDigest, req.URL.Path)
		default:
			writeErrorResponse(w, req, InternalError, req.URL.Path)
		}
		return
	}
	w.Header().Set("ETag", "\""+metadata.Md5+"\"")
	writeSuccessResponse(w)
}

// PutBucketACLHandler - PUT Bucket ACL
// ----------
// This implementation of the PUT operation modifies the bucketACL for authenticated request
func (api CloudStorageAPI) PutBucketACLHandler(w http.ResponseWriter, req *http.Request) {
	vars := mux.Vars(req)
	bucket := vars["bucket"]

	if !api.Anonymous {
		if isRequestRequiresACLCheck(req) {
			writeErrorResponse(w, req, AccessDenied, req.URL.Path)
			return
		}
	}

	// read from 'x-amz-acl'
	aclType := getACLType(req)
	if aclType == unsupportedACLType {
		writeErrorResponse(w, req, NotImplemented, req.URL.Path)
		return
	}
	err := api.Filesystem.SetBucketMetadata(bucket, map[string]string{"acl": getACLTypeString(aclType)})
	if err != nil {
		errorIf(err.Trace(), "PutBucketACL failed.", nil)
		switch err.ToGoError().(type) {
		case fs.BucketNameInvalid:
			writeErrorResponse(w, req, InvalidBucketName, req.URL.Path)
		case fs.BucketNotFound:
			writeErrorResponse(w, req, NoSuchBucket, req.URL.Path)
		default:
			writeErrorResponse(w, req, InternalError, req.URL.Path)
		}
		return
	}
	writeSuccessResponse(w)
}

// GetBucketACLHandler - GET ACL on a Bucket
// ----------
// This operation uses acl subresource to the return the ``acl``
// of a bucket. One must have permission to access the bucket to
// know its ``acl``. This operation willl return response of 404
// if bucket not found and 403 for invalid credentials.
func (api CloudStorageAPI) GetBucketACLHandler(w http.ResponseWriter, req *http.Request) {
	vars := mux.Vars(req)
	bucket := vars["bucket"]

	if !api.Anonymous {
		if isRequestRequiresACLCheck(req) {
			writeErrorResponse(w, req, AccessDenied, req.URL.Path)
			return
		}
	}

	bucketMetadata, err := api.Filesystem.GetBucketMetadata(bucket)
	if err != nil {
		errorIf(err.Trace(), "GetBucketMetadata failed.", nil)
		switch err.ToGoError().(type) {
		case fs.BucketNotFound:
			writeErrorResponse(w, req, NoSuchBucket, req.URL.Path)
		case fs.BucketNameInvalid:
			writeErrorResponse(w, req, InvalidBucketName, req.URL.Path)
		default:
			writeErrorResponse(w, req, InternalError, req.URL.Path)
		}
		return
	}
	// generate response
	response := generateAccessControlPolicyResponse(bucketMetadata.ACL)
	encodedSuccessResponse := encodeSuccessResponse(response)
	// write headers
	setCommonHeaders(w, len(encodedSuccessResponse))
	// write body
	w.Write(encodedSuccessResponse)
}

// HeadBucketHandler - HEAD Bucket
// ----------
// This operation is useful to determine if a bucket exists.
// The operation returns a 200 OK if the bucket exists and you
// have permission to access it. Otherwise, the operation might
// return responses such as 404 Not Found and 403 Forbidden.
func (api CloudStorageAPI) HeadBucketHandler(w http.ResponseWriter, req *http.Request) {
	vars := mux.Vars(req)
	bucket := vars["bucket"]

	if !api.Anonymous {
		if isRequestRequiresACLCheck(req) {
			if api.Filesystem.IsPrivateBucket(bucket) {
				writeErrorResponse(w, req, AccessDenied, req.URL.Path)
				return
			}
		}
	}

	_, err := api.Filesystem.GetBucketMetadata(bucket)
	if err != nil {
		errorIf(err.Trace(), "GetBucketMetadata failed.", nil)
		switch err.ToGoError().(type) {
		case fs.BucketNotFound:
			writeErrorResponse(w, req, NoSuchBucket, req.URL.Path)
		case fs.BucketNameInvalid:
			writeErrorResponse(w, req, InvalidBucketName, req.URL.Path)
		default:
			writeErrorResponse(w, req, InternalError, req.URL.Path)
		}
		return
	}
	writeSuccessResponse(w)
}

// DeleteBucketHandler - Delete bucket
func (api CloudStorageAPI) DeleteBucketHandler(w http.ResponseWriter, req *http.Request) {
	vars := mux.Vars(req)
	bucket := vars["bucket"]

	if !api.Anonymous {
		if isRequestRequiresACLCheck(req) {
			writeErrorResponse(w, req, AccessDenied, req.URL.Path)
			return
		}
	}

	err := api.Filesystem.DeleteBucket(bucket)
	if err != nil {
		errorIf(err.Trace(), "DeleteBucket failed.", nil)
		switch err.ToGoError().(type) {
		case fs.BucketNotFound:
			writeErrorResponse(w, req, NoSuchBucket, req.URL.Path)
		case fs.BucketNotEmpty:
			writeErrorResponse(w, req, BucketNotEmpty, req.URL.Path)
		default:
			writeErrorResponse(w, req, InternalError, req.URL.Path)
		}
		return
	}
	writeSuccessNoContent(w)
}

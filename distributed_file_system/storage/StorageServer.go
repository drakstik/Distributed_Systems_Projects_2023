package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"

	// "strconv"
	"encoding/base64"
	"errors"
)

/* Start of Global Constants */

const PROTOCOL string = "tcp"
const NAMING_SERVER_IP string = "http://127.0.0.1:"
const STORAGE_IP string = "http://127.0.0.1:"

const FILE_PERMISSIONS = 0644

/* API Endpoints */
const REGISTRATION_API_ENDPOINT string = "/register"
const STORAGE_SIZE_API_ENDPOINT string = "/storage_size"
const STORAGE_READ_API_ENDPOINT string = "/storage_read"
const STORAGE_WRITE_API_ENDPOINT string = "/storage_write"
const STORAGE_CREATE_API_ENDPOINT string = "/storage_create"
const STORAGE_DELETE_API_ENDPOINT string = "/storage_delete"
const STORAGE_COPY_API_ENDPOINT string = "/storage_copy"

/* End of Global Constants */

var STORAGE_OUT os.File

type ExceptionResponse struct {
	ExceptionType string `json:"exception_type"`
	ExceptionInfo string `json:"exception_info"`
}

type FileList struct {
	Files []string `json:"files"`
}

type StorageServer struct {
	clientPort       string
	commandPort      string
	registrationPort string
	root             string
}

type RegisterRequest struct {
	Storage_IP  string   `json:"storage_ip"`
	ClientPort  string   `json:"client_port"`
	CommandPort string   `json:"command_port"`
	Files       []string `json:"files"`
}

type StorageSizeRequest struct {
	Path string `json:"path"`
}

type StorageSizeResponse struct {
	Size int `json:"size"`
}

type StorageReadRequest struct {
	Path   string `json:"path"`
	Offset int    `json:"offset"`
	Length int    `json:"length"`
}

type StorageReadResponse struct {
	Data string `json:"data"`
}

type StorageWriteRequest struct {
	Path   string `json:"path"`
	Offset int    `json:"offset"`
	Data   string `json:"data"`
}

type StorageWriteResponse struct {
	Success bool `json:"success"`
}

type StorageCreateRequest struct {
	Path string `json:"path"`
}

type StorageCreateResponse struct {
	Success bool `json:"success"`
}

type StorageDeleteRequest struct {
	Path string `json:"path"`
}

type StorageDeleteResponse struct {
	Success bool `json:"success"`
}

type StorageCopyRequest struct {
	Path       string `json:"path"`
	ServerIP   string `json:"server_ip"`
	ServerPort int    `json:"server_port"`
}

type StorageCopyResponse struct {
	Success bool `json:"success"`
}

func (storageServer *StorageServer) HandleInvalidRequestParams(
	w http.ResponseWriter,
	r *http.Request,
	path string,
	offset int,
	readLength int,
	API string) bool {
	/* Check for null arguments in HTTP Request */
	response := ExceptionResponse{}
	if path == "" {
		// File does not exist, handle error
		response.ExceptionType = "IllegalArgumentException"
		response.ExceptionInfo = "No arguments passed in the API request body"
		json.NewEncoder(w).Encode(response)
		fmt.Fprintln(&STORAGE_OUT, "Storage Server Response:", response)
		return true
	}

	filePath := filepath.Join(storageServer.root, path)
	fileInfo, err := os.Stat(filePath)

	create := API == STORAGE_CREATE_API_ENDPOINT
	delete := API == STORAGE_DELETE_API_ENDPOINT
	copy := API == STORAGE_COPY_API_ENDPOINT

	/* Check if this file exists or if this is a directory */
	if !create && !copy && (os.IsNotExist(err) || (!delete && fileInfo.IsDir())) {
		// File does not exist, handle error
		response.ExceptionType = "FileNotFoundException"
		response.ExceptionInfo = "The file does not exist on storage server"
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(response)
		fmt.Fprintln(&STORAGE_OUT, "Storage Response:", response)
		return true
	}

	read := API == STORAGE_READ_API_ENDPOINT

	invalidLength := fileInfo != nil && (readLength < 0 || int64(readLength) > fileInfo.Size())
	invalidOffset_read := read && (fileInfo.Size() != 0 && (offset < 0 || int64(offset) >= fileInfo.Size()))
	invalidOffset_write := offset < 0

	/* Check for negative offset values */
	if invalidLength || invalidOffset_read || invalidOffset_write {
		response.ExceptionType = "IndexOutOfBoundsException"
		response.ExceptionInfo = "Invalid Offset value supplied in Storage Write Request"
		json.NewEncoder(w).Encode(response)
		fmt.Fprintln(&STORAGE_OUT, "Storage Response:", response)
		return true
	}

	return false
}

func (storageServer *StorageServer) HandleStorageSizeRequest(w http.ResponseWriter, r *http.Request) {
	var req StorageSizeRequest
	decode_err := json.NewDecoder(r.Body).Decode(&req)
	if decode_err != nil {
		fmt.Fprintf(&STORAGE_OUT, "Storage: Decoding Error: %v\n", decode_err)
	}

	invalidRequestParams := storageServer.HandleInvalidRequestParams(w, r, req.Path, 0, 0, STORAGE_SIZE_API_ENDPOINT)

	if invalidRequestParams {
		return
	}

	filePath := filepath.Join(storageServer.root, req.Path)
	fileInfo, _ := os.Stat(filePath)
	fmt.Fprintln(&STORAGE_OUT, "Client Requested File Information for : %v\n", filePath)

	/* Return the size of the valid file */
	response := StorageSizeResponse{
		Size: int(fileInfo.Size()),
	}
	json.NewEncoder(w).Encode(response)
	fmt.Fprintln(&STORAGE_OUT, "Storage Size Response:", response)
	return
}

func (storageServer *StorageServer) HandleStorageReadRequest(w http.ResponseWriter, r *http.Request) {
	var req StorageReadRequest
	decode_err := json.NewDecoder(r.Body).Decode(&req)
	if decode_err != nil {
		fmt.Fprintf(&STORAGE_OUT, "Storage: Decoding Error: %v\n", decode_err)
	}
	fmt.Fprintf(&STORAGE_OUT, "Storage: New SR Request: %v\n", req)
	invalidRequestParams := storageServer.HandleInvalidRequestParams(w, r, req.Path, req.Offset, req.Length, STORAGE_READ_API_ENDPOINT)

	if invalidRequestParams {
		return
	}

	filePath := filepath.Join(storageServer.root, req.Path)

	/* Return the contents of the valid file */
	data, read_err := os.ReadFile(filePath)
	if read_err != nil {
		fmt.Fprintf(&STORAGE_OUT, "Storage: Error Reading Contents from File: %v\n", read_err)
		return
	}

	data = data[req.Offset : req.Offset+req.Length]

	response := StorageReadResponse{
		Data: base64.StdEncoding.EncodeToString(data),
	}

	json.NewEncoder(w).Encode(response)
	fmt.Fprintln(&STORAGE_OUT, "Storage Read Response:", response)
	return

}

func (storageServer *StorageServer) HandleStorageWriteRequest(w http.ResponseWriter, r *http.Request) {
	var req StorageWriteRequest
	decode_err := json.NewDecoder(r.Body).Decode(&req)
	if decode_err != nil {
		fmt.Fprintf(&STORAGE_OUT, "Storage: Decoding Error: %v\n", decode_err)
	}

	invalidRequestParams := storageServer.HandleInvalidRequestParams(w, r, req.Path, req.Offset, 0, STORAGE_WRITE_API_ENDPOINT)

	if invalidRequestParams {
		return
	}

	filePath := filepath.Join(storageServer.root, req.Path)

	response := StorageWriteResponse{}

	/* Open the file */
	file, open_err := os.OpenFile(filePath, os.O_WRONLY|os.O_CREATE, FILE_PERMISSIONS)

	if open_err != nil {
		fmt.Fprintf(&STORAGE_OUT, "Storage: PrajneyaError Opening File: %v\n", open_err)
		response.Success = false
	}
	defer file.Close()

	_, write_err := file.Seek(int64(req.Offset), io.SeekStart)
	if write_err != nil {
		fmt.Fprintf(&STORAGE_OUT, "Storage: Error Seeking Offset for File: %v\n", write_err)
		response.Success = false
	}

	/* Write the contents of the request to the valid file */
	fmt.Fprintf(&STORAGE_OUT, "Storage: Request Body: %v\n", req)

	base64RequestString, encoding_err := base64.StdEncoding.DecodeString(req.Data)
	if encoding_err != nil {
		fmt.Fprintf(&STORAGE_OUT, "Storage: Error Encoding Base64 String %v\n", encoding_err)
		return
	}

	data := []byte(base64RequestString)

	_, write_err = file.Write(data)
	if write_err != nil {
		fmt.Fprintf(&STORAGE_OUT, "Storage: Error Writing Contents to File: %v\n", write_err)
		response.Success = false
	}

	response.Success = true

	json.NewEncoder(w).Encode(response)
	fmt.Fprintln(&STORAGE_OUT, "Storage Write Response:", response)
	return

}

func (storageServer *StorageServer) HandleStorageCreateRequest(w http.ResponseWriter, r *http.Request) {
	var req StorageCreateRequest
	decode_err := json.NewDecoder(r.Body).Decode(&req)
	if decode_err != nil {
		fmt.Fprintf(&STORAGE_OUT, "Storage: Decoding Error: %v\n", decode_err)
	}
	invalidRequestParams := storageServer.HandleInvalidRequestParams(w, r, req.Path, 0, 0, STORAGE_CREATE_API_ENDPOINT)

	if invalidRequestParams {
		return
	}

	filePath := filepath.Join(storageServer.root, req.Path)
	_, err := os.Stat(filePath)

	/* Create a new file */
	response := StorageCreateResponse{}

	if !os.IsNotExist(err) {
		fmt.Fprintf(&STORAGE_OUT, "Storage: Error Creating New File: File with same name already exists\n")
		response.Success = false
	} else {
		/* Create all the directories in the path */
		mkdir_err := os.MkdirAll(filepath.Dir(filePath), os.ModePerm)
		if mkdir_err != nil {
			fmt.Fprintf(&STORAGE_OUT, "Storage: Error Creating New Directories: %v\n", mkdir_err)
			response.Success = false
		} else {
			file, create_err := os.Create(filePath)
			if create_err != nil {
				fmt.Fprintf(&STORAGE_OUT, "Storage: Error Creating New File: %v\n", create_err)
				response.Success = false
			} else {
				response.Success = true
				file.Close()
			}
		}
	}

	json.NewEncoder(w).Encode(response)
	fmt.Fprintln(&STORAGE_OUT, "Storage Create Response:", response)
	return

}

func (storageServer *StorageServer) HandleStorageDeleteRequest(w http.ResponseWriter, r *http.Request) {
	var req StorageDeleteRequest
	decode_err := json.NewDecoder(r.Body).Decode(&req)
	if decode_err != nil {
		fmt.Fprintf(&STORAGE_OUT, "Storage: Decoding Error: %v\n", decode_err)
	}
	fmt.Fprintf(&STORAGE_OUT, "Storage: New Delete Request: %v\n", req)
	invalidRequestParams := storageServer.HandleInvalidRequestParams(w, r, req.Path, 0, 0, STORAGE_DELETE_API_ENDPOINT)

	if invalidRequestParams {
		return
	}

	filePath := filepath.Join(storageServer.root, req.Path)
	fileInfo, _ := os.Stat(filePath)

	/* Remove the file */
	response := StorageDeleteResponse{}

	remove_err := errors.New("")

	if fileInfo.IsDir() && filePath != storageServer.root {
		remove_err = os.RemoveAll(filePath)
	} else {
		remove_err = os.Remove(filePath)
	}

	if remove_err != nil {
		fmt.Fprintf(&STORAGE_OUT, "Storage: Error Deleting File: %v\n", remove_err)
		response.Success = false
	} else {
		response.Success = true
	}

	storageServer.RecursivelyDeleteEmptyDirs()

	json.NewEncoder(w).Encode(response)
	fmt.Fprintln(&STORAGE_OUT, "Storage Delete Response:", response)
	return
}

func (storageServer *StorageServer) HandleStorageCopyRequest(w http.ResponseWriter, r *http.Request) {
	var req StorageCopyRequest
	decode_err := json.NewDecoder(r.Body).Decode(&req)
	if decode_err != nil {
		fmt.Fprintf(&STORAGE_OUT, "Storage: Decoding Error: %v\n", decode_err)
	}
	fmt.Fprintf(&STORAGE_OUT, "Storage: New Copy Request: %v\n", req)
	invalidRequestParams := storageServer.HandleInvalidRequestParams(w, r, req.Path, 0, 0, STORAGE_COPY_API_ENDPOINT)

	if invalidRequestParams {
		return
	}

	var response StorageCopyResponse
	response.Success = false

	/* Get the size of the file requested to be copied */
	client := &http.Client{}
	STORAGE_SERVER_SIZE_URL := fmt.Sprintf("%v%v%v",
		req.ServerIP,
		req.ServerPort,
		STORAGE_SIZE_API_ENDPOINT,
	)

	length := 0

	/* Send an HTTP Request to read the size of the file */
	sizeRequest := StorageSizeRequest{
		Path: req.Path,
	}

	payload, err := json.Marshal(sizeRequest)
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(response)
		fmt.Fprintf(&STORAGE_OUT, "Storage: Error Encoding JSON: %v\n", err)
		return
	}

	http_req, http_err := http.NewRequest("POST", STORAGE_SERVER_SIZE_URL, bytes.NewBuffer(payload))
	if http_err != nil {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(response)
		fmt.Fprintln(&STORAGE_OUT, "Storage: Error Creating Storage Size HTTP Request", http_err)
		return
	}

	/* Make Size Request API Call */
	http_res, http_err := client.Do(http_req)
	if http_err != nil {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(response)
		fmt.Fprintf(&STORAGE_OUT, "Storage: Error Sending Storage Size HTTP Request %v\n", http_err)
		return
	} else {
		// Read the response body
		defer http_res.Body.Close()
		body, body_err := ioutil.ReadAll(http_res.Body)
		if body_err != nil {
			fmt.Fprintln(&STORAGE_OUT, "Storage: Error Reading Storage Size HTTP Response")
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(response)
			return
		}
		// Print the response body
		fmt.Fprintln(&STORAGE_OUT, "Storage Size Response: "+string(body))
		fmt.Fprintf(&STORAGE_OUT, "Storage Size Response Code: %v\n", http_res.StatusCode)

		/* File does not exist */
		if http_res.StatusCode == 404 {
			storageServer.HandleInvalidRequestParams(w, r, "invalid_path", 0, 0, STORAGE_SIZE_API_ENDPOINT)
			return
		}

		// Handle the response
		var res StorageSizeResponse
		decode_err := json.Unmarshal(body, &res)
		if decode_err != nil {
			fmt.Fprintf(&STORAGE_OUT, "Storage: Decoding Error: %v\n", decode_err)
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(response)
			return
		}
		length = res.Size
	}

	client = &http.Client{}
	STORAGE_SERVER_READ_URL := fmt.Sprintf("%v%v%v",
		req.ServerIP,
		req.ServerPort,
		STORAGE_READ_API_ENDPOINT,
	)

	/* Send an HTTP Request to read the contents of the file */
	readRequest := StorageReadRequest{
		Path:   req.Path,
		Offset: 0,
		Length: length,
	}

	// Create a Read Request to Storage Server
	payload, err = json.Marshal(readRequest)
	if err != nil {
		fmt.Fprintf(&STORAGE_OUT, "Storage: Error Encoding JSON: %v\n", err)
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(response)
		return
	}

	http_req, http_err = http.NewRequest("POST", STORAGE_SERVER_READ_URL, bytes.NewBuffer(payload))
	if http_err != nil {
		fmt.Fprintln(&STORAGE_OUT, "Storage: Error Creating Storage Read HTTP Request", http_err)
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(response)
		return
	}

	// Send the request
	http_res, http_err = client.Do(http_req)
	if http_err != nil {
		fmt.Fprintf(&STORAGE_OUT, "Storage: Error Sending Storage Read HTTP Request %v\n", http_err)
		response.Success = false
	} else {
		// Read the response body
		defer http_res.Body.Close()
		body, body_err := ioutil.ReadAll(http_res.Body)
		if body_err != nil {
			fmt.Fprintln(&STORAGE_OUT, "Storage: Error Reading Storage Read HTTP Response")
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(response)
			return
		}

		// Handle the response
		var res StorageReadResponse
		decode_err := json.Unmarshal(body, &res)
		if decode_err != nil {
			fmt.Fprintf(&STORAGE_OUT, "Storage: Decoding Error: %v\n", decode_err)
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(response)
			return
		} else {
			/* Write the contents of the request to the valid file */
			fmt.Fprintf(&STORAGE_OUT, "Storage: Response Body: %v\n", res)

			base64RequestString, encoding_err := base64.StdEncoding.DecodeString(res.Data)
			if encoding_err != nil {
				fmt.Fprintf(&STORAGE_OUT, "Storage: Error Encoding Base64 String %v\n", encoding_err)
				w.WriteHeader(http.StatusNotFound)
				json.NewEncoder(w).Encode(response)
				return
			}

			fmt.Fprintf(&STORAGE_OUT, "Storage: Response base64RequestString: %v\n", string(base64RequestString))

			normalString := string(base64RequestString)
			fmt.Fprintf(&STORAGE_OUT, "Storage: Response Normal String: %v\n", normalString)

			/* Create the file and all directories leading to it. Overwriting the file if it exists */
			filePath := filepath.Join(storageServer.root, req.Path)
			_, fileErr := os.Stat(filePath)

			if !os.IsNotExist(fileErr) {
				fmt.Fprintf(&STORAGE_OUT, "Storage: File %v already existed, removing ... :\n", filePath)
				os.Remove(filePath)
			}

			storageServer.RecursivelyDeleteEmptyDirs()

			mkdir_err := os.MkdirAll(filepath.Dir(filePath), os.ModePerm)
			if mkdir_err != nil {
				fmt.Fprintf(&STORAGE_OUT, "Storage: Error Creating New Directories: %v\n", mkdir_err)
				w.WriteHeader(http.StatusNotFound)
				json.NewEncoder(w).Encode(response)
			} else {
				file, file_create_err := os.Create(filePath)
				if file_create_err != nil {
					fmt.Fprintf(&STORAGE_OUT, "Storage: Error Creating New File: %v\n", file_create_err)
					w.WriteHeader(http.StatusNotFound)
					json.NewEncoder(w).Encode(response)
				} else {
					file.Close()
				}
			}

			/* Open the file */
			fmt.Fprintf(&STORAGE_OUT, "Storage: Creating/Overwriting File %v\n", filePath)

			file, open_err := os.OpenFile(filePath, os.O_WRONLY|os.O_CREATE, 0644)

			/* Write the contents to the file */
			if open_err != nil {
				fmt.Fprintf(&STORAGE_OUT, "Storage: Error Opening File: %v\n", open_err)
				w.WriteHeader(http.StatusNotFound)
				json.NewEncoder(w).Encode(response)
				return
			}

			data := []byte(normalString)

			_, write_err := file.Write(data)
			if write_err != nil {
				fmt.Fprintf(&STORAGE_OUT, "Storage: Error Writing Contents to File: %v\n", write_err)
				w.WriteHeader(http.StatusNotFound)
				json.NewEncoder(w).Encode(response)
				return
			}
		}
	}
	response.Success = true
	json.NewEncoder(w).Encode(response)
	fmt.Fprintln(&STORAGE_OUT, "Storage Copy Response:", response)
	return
}

func (storageServer *StorageServer) HandleHTTPRequest(w http.ResponseWriter, r *http.Request) {
	switch r.RequestURI {
	case STORAGE_SIZE_API_ENDPOINT:
		storageServer.HandleStorageSizeRequest(w, r)
	case STORAGE_READ_API_ENDPOINT:
		storageServer.HandleStorageReadRequest(w, r)
	case STORAGE_WRITE_API_ENDPOINT:
		storageServer.HandleStorageWriteRequest(w, r)
	case STORAGE_CREATE_API_ENDPOINT:
		storageServer.HandleStorageCreateRequest(w, r)
	case STORAGE_DELETE_API_ENDPOINT:
		storageServer.HandleStorageDeleteRequest(w, r)
	case STORAGE_COPY_API_ENDPOINT:
		storageServer.HandleStorageCopyRequest(w, r)
	default:
		return
	}
}

func isDirEmpty(dirname string) bool {
	f, err := os.Open(dirname)
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()

	_, err = f.Readdir(1)
	if err == io.EOF {
		return true
	}
	return false
}

func (storageServer *StorageServer) RecursivelyDeleteEmptyDirs() {
	var emptyDirs []string

	// Print the names of the files
	filepath.Walk(storageServer.root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			empty := isDirEmpty(path)
			if empty {
				emptyDirs = append(emptyDirs, path)
			}
		}
		return nil
	})

	fmt.Fprintf(&STORAGE_OUT, "Empty Directory List : %v\n", emptyDirs)

	if len(emptyDirs) == 0 {
		return
	}

	// Delete the empty directories
	for _, dir := range emptyDirs {
		err := os.Remove(dir)
		if err != nil {
			fmt.Fprintf(&STORAGE_OUT, "Can't delete directory : %v\n", err)
		} else {
			fmt.Fprintf(&STORAGE_OUT, "Directory deleted : %v\n", dir)
		}
	}

	storageServer.RecursivelyDeleteEmptyDirs()

}

/* Delete Files as directed by Naming Server */
func (storageServer *StorageServer) DeleteFiles(fileList FileList) {
	fmt.Fprintf(&STORAGE_OUT, "Storage: Deleting these file from %v:%v", storageServer.root, fileList)
	for _, file := range fileList.Files {
		filePath := filepath.Join(storageServer.root, file)
		if _, err := os.Stat(filePath); os.IsNotExist(err) {
			// File does not exist, handle error or skip
			fmt.Fprintf(&STORAGE_OUT, "Storage: File %v does on exist on %v", file, storageServer.root)
			continue
		}
		err := os.Remove(filePath)
		if err != nil {
			fmt.Fprintf(&STORAGE_OUT, "Storage: Unable to delete %v: %v", file, err)
			continue
		}
		fmt.Fprintf(&STORAGE_OUT, "Deleted File %v\n", filePath)
	}

	storageServer.RecursivelyDeleteEmptyDirs()

}

func (storageServer *StorageServer) Register() {

	fmt.Fprintln(&STORAGE_OUT, "Storage: Sending HTTP Request")

	/* Register the storage server */

	// Create an HTTP client
	client := &http.Client{}

	NAMING_SERVER_ADDRESS := fmt.Sprintf("%v%v%v",
		STORAGE_IP,
		storageServer.registrationPort,
		REGISTRATION_API_ENDPOINT,
	)

	fileList := []string{}

	// Print the names of the files
	err := filepath.Walk(storageServer.root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			relPath, err := filepath.Rel(storageServer.root, path)
			if err != nil {
				fmt.Fprintf(&STORAGE_OUT, "Storage: Error finding the rel path of file %v\n", path)
				return nil
			}
			relPath = fmt.Sprintf("/%v", relPath)
			fileList = append(fileList, relPath)
			fmt.Fprintf(&STORAGE_OUT, "Found File : %v\n", path)
		}
		return nil
	})

	if err != nil {
		fmt.Fprintf(&STORAGE_OUT, "Error in Path Walk: %v\n", err)
	}

	fmt.Fprintf(&STORAGE_OUT, "Current List of Files : %v\n", fileList)

	registerRequest := RegisterRequest{
		Storage_IP:  STORAGE_IP,
		ClientPort:  storageServer.clientPort,
		CommandPort: storageServer.commandPort,
		Files:       fileList,
	}

	// Create a GET request to Naming Server
	payload, err := json.Marshal(registerRequest)
	if err != nil {
		fmt.Fprintf(&STORAGE_OUT, "Storage: Error Encoding JSON: %v\n", err)
	}
	req, err := http.NewRequest("POST", NAMING_SERVER_ADDRESS, bytes.NewBuffer(payload))
	fmt.Fprintf(&STORAGE_OUT, "%q\n", NAMING_SERVER_ADDRESS)
	if err != nil {
		fmt.Fprintln(&STORAGE_OUT, "Storage: Error Creating Registration HTTP Request", err)
	}

	// Send the request
	resp, err := client.Do(req)
	if err != nil {
		fmt.Fprintf(&STORAGE_OUT, "Storage: Error Sending Registration HTTP Request %v\n", err)
	} else {
		// Read the response body
		defer resp.Body.Close()
		body, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			fmt.Fprintln(&STORAGE_OUT, "Storage: Error Reading Registration HTTP Response")
		}
		// Print the response body
		fmt.Fprintln(&STORAGE_OUT, "Registration Response: "+string(body))

		// Handle the response
		var filesToBeDeleted FileList
		decode_err := json.Unmarshal(body, &filesToBeDeleted)
		if decode_err != nil {
			fmt.Fprintf(&STORAGE_OUT, "Storage: Decoding Error: %v\n", decode_err)
			return
		}
		fmt.Fprintln(&STORAGE_OUT, "Registration Complete, Sending Files for Deletion "+string(body))
		fmt.Fprintln(&STORAGE_OUT, "Decoded =  ", filesToBeDeleted)

		storageServer.DeleteFiles(filesToBeDeleted)
	}
}

func (storageServer *StorageServer) ServeClient(clientListener *net.Listener) {
	CLIENT_ADDRESS := "127.0.0.1:" + storageServer.clientPort
	/* Wrapper Function to Handle HTTP Requests */
	handler := func(w http.ResponseWriter, r *http.Request) {
		storageServer.HandleHTTPRequest(w, r)
	}
	client_err := http.Serve(*clientListener, http.HandlerFunc(handler))
	if client_err != nil {
		fmt.Fprintln(&STORAGE_OUT, "Storage: Error Serving HTTP on CLT PORT")
	}
	fmt.Fprintf(&STORAGE_OUT, "Client Interface has started on %v", CLIENT_ADDRESS)
}

func (storageServer *StorageServer) ServeCommand(commandListener *net.Listener) {
	COMMAND_ADDRESS := "127.0.0.1:" + storageServer.commandPort
	handler := func(w http.ResponseWriter, r *http.Request) {
		storageServer.HandleHTTPRequest(w, r)
	}
	command_err := http.Serve(*commandListener, http.HandlerFunc(handler))
	if command_err != nil {
		fmt.Fprintln(&STORAGE_OUT, "Storage: Error Serving HTTP on CMD PORT")
	}
	fmt.Fprintf(&STORAGE_OUT, "Command Interface has started on %v", COMMAND_ADDRESS)
}

/* Start the Storage Server */
func (storageServer *StorageServer) Start() {

	CLIENT_ADDRESS := "127.0.0.1:" + storageServer.clientPort
	COMMAND_ADDRESS := "127.0.0.1:" + storageServer.commandPort

	clientListener, err := net.Listen(PROTOCOL, CLIENT_ADDRESS)
	if err != nil {
		fmt.Fprintln(&STORAGE_OUT, "Error Starting CLIENT_ADDRESS Server")
	}

	commandListener, err := net.Listen(PROTOCOL, COMMAND_ADDRESS)
	if err != nil {
		fmt.Fprintln(&STORAGE_OUT, "Error Starting COMMAND_ADDRESS Server")
	}

	fmt.Fprintln(&STORAGE_OUT, "Listening on ", CLIENT_ADDRESS)
	fmt.Fprintln(&STORAGE_OUT, "Listening on ", COMMAND_ADDRESS)

	/* Accept HTTP Requests */
	go storageServer.ServeClient(&clientListener)
	go storageServer.ServeCommand(&commandListener)
	for {
	}
}

/* Start the Storage Server */
func main() {
	/* Create a new file output to storage service logs. */
	file, err := os.OpenFile("storage_output.txt", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		log.Println(err)
	}

	defer file.Close()

	/* Clear Storage Output File */
	_, err = file.WriteString("")
	if err != nil {
		fmt.Println(err)
	}

	STORAGE_OUT = *file

	fmt.Fprintln(&STORAGE_OUT, "Storage Server is starting")

	// Get arguments from Storage Command Argument (StorageCommands.java: storage0Command)
	args := os.Args[1:] // Get args
	fmt.Fprintln(&STORAGE_OUT, args)

	CLIENT_PORT := args[0]
	COMMAND_PORT := args[1]
	REGISTRATION_PORT := args[2]

	STORAGE_ROOT := args[3]

	storageServer := &StorageServer{clientPort: CLIENT_PORT,
		commandPort:      COMMAND_PORT,
		registrationPort: REGISTRATION_PORT,
		root:             STORAGE_ROOT,
	}
	storageServer.Register()
	storageServer.Start()
}

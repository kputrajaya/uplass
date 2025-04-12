# Uplass

Asset upload API, storage, and delivery for multiple apps.

## Creating an App

To use Uplass, setup the environment variables (`.env`) then create an app.

```sh
$ go run commands/create_app/main.go [app_key] [app_secret]
```

## Uploading a File

To upload a file, get an upload token first.

```py
url = 'https://uplass.dummy/token'
data = {'AppKey': app_key, 'AppSecret': app_secret}
response = requests.post(url, json=data)
token = response.text
```

Then, use the token to upload the file.

```py
url = 'https://uplass.dummy/upload'
data = {'token': token}
files = {'file': (uploaded.filename, uploaded.file.read(), uploaded.type)}
response = requests.post(url, data=data, files=files)
image_url = response.text
```

## Built With

- [Go Fiber](https://gofiber.io/)
- [GORM](https://gorm.io/index.html)
- [AWS SDK for Go](https://github.com/aws/aws-sdk-go)

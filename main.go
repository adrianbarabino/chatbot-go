package main

// Inicializar la base de datos al inicio de la aplicación

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"time"

	"github.com/joho/godotenv"

	_ "github.com/mattn/go-sqlite3"
)

// Definiendo la estructura de las plantillas de mensajes

type MessageTemplate struct {
	ID      string `json:"id"`
	Message string `json:"message"`
}

var messageTemplates []MessageTemplate

// Constantes para la aplicación
var (
	verifyToken         string
	whatsappUrl         string
	whatsappBusinessUrl string
	myPhoneID           string
	whatsappToken       string
	port                string
)

const (
	// Nombres de las tablas en la base de datos

	usuariosTabla = "usuarios"
	mensajesTabla = "mensajes"

	// Los estados de la aplicación
	// Estos son importantes para el flujo de la conversación
	// Ya que almacenamos el estado actual del usuario en la base de datos

	estadoPrincipal = "MENU_PRINCIPAL"
	estadoTours     = "TOURS"
	estadoTraslados = "TRASLADOS"
	estadoAgente    = "AGENTE"
)

var db *sql.DB

func inicializarBaseDeDatos() error {
	var err error

	// Actualmente la base de datos es SQLite
	// Pero después vamos a actualizar a una con mejor randimiento y escalabilidad
	// como por ejemplo PostgreSQL o MySQL

	db, err = sql.Open("sqlite3", "./base_de_datos.db")
	if err != nil {
		return err
	}

	// Crear tabla para almacenar estados de usuarios
	// A futuro vamos a necesitar más tablas para almacenar otros datos
	// como por ejemplo, los datos de los usuarios, los mensajes enviados, etc.

	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS ` + usuariosTabla + ` (
			numero TEXT PRIMARY KEY,
			estado TEXT,
			fecha_actualizacion DATE
		);
	`)
	if err != nil {
		return err
	}

	// Crear tabla para almacenar mensajes
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS ` + mensajesTabla + ` (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			numero TEXT,
			tipo TEXT,
			mensaje TEXT,
			timestamp TEXT
		);
	`)
	if err != nil {
		return err
	}

	return nil
}

func cerrarBaseDeDatos() {
	// Esta función se ejecuta al final de la aplicación
	// para cerrar la conexión con la base de datos

	if db != nil {
		db.Close()
	}
}

func main() {
	err := godotenv.Load()
	if err != nil {
		fmt.Println("Error al cargar el archivo .env")
	}

	verifyToken = os.Getenv("VERIFY_TOKEN")
	whatsappUrl = os.Getenv("WHATSAPP_URL")
	whatsappBusinessUrl = os.Getenv("WHATSAPP_BUSINESS_URL")
	myPhoneID = os.Getenv("MY_PHONE_ID")
	whatsappToken = os.Getenv("WHATSAPP_TOKEN")
	port = os.Getenv("PORT")
	// Inicializar la base de datos al inicio de la aplicación
	if err := inicializarBaseDeDatos(); err != nil {
		fmt.Println("Error al inicializar la base de datos:", err)
		return
	}

	// Payload vacío
	payload := []byte(`{}`)

	// Crear la solicitud HTTP GET
	req, err := http.NewRequest("GET", whatsappBusinessUrl, bytes.NewBuffer(payload))
	if err != nil {
		fmt.Println("Error al crear la solicitud HTTP:", err)
		return
	}

	// Agregar encabezados necesarios
	req.Header.Set("Authorization", "Bearer "+whatsappToken)
	req.Header.Set("Content-Type", "application/json")

	// Realizar la solicitud HTTP
	// Esto es para obtener las plantillas de mensajes
	// que vamos a utilizar en la aplicación

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		fmt.Println("Error al realizar la solicitud HTTP:", err)
		return
	}
	defer resp.Body.Close()

	// Decodificar la respuesta JSON
	// y guardar las plantillas en un slice

	var data map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&data)
	if err != nil {
		fmt.Println("Error al decodificar JSON:", err)
		return
	}

	// Obtener el array de "data"
	templates, ok := data["data"].([]interface{})
	if !ok {
		fmt.Println("Error al obtener los templates")
		return
	}

	// Iterar sobre las plantillas y guardar "id" y "message"
	for _, template := range templates {
		templateMap, ok := template.(map[string]interface{})
		if !ok {
			fmt.Println("Error al convertir la plantilla a mapa")
			return
		}

		id, ok := templateMap["name"].(string)
		if !ok {
			fmt.Println("Error al obtener el ID de la plantilla")
			return
		}

		components, ok := templateMap["components"].([]interface{})
		if !ok {
			fmt.Println("Error al obtener los componentes de la plantilla")
			return
		}

		// Obtener el texto del componente BODY
		var message string
		for _, component := range components {
			componentMap, ok := component.(map[string]interface{})
			if !ok {
				fmt.Println("Error al convertir el componente a mapa")
				return
			}

			if componentMap["type"] == "BODY" {
				message, ok = componentMap["text"].(string)
				if !ok {
					fmt.Println("Error al obtener el texto del componente BODY")
					return
				}
				break
			}
		}

		// Guardar en el slice de plantillas
		messageTemplates = append(messageTemplates, MessageTemplate{ID: id, Message: message})
		// Asi que en messageTemplates tendriamos algo como:
		// [ { "id": "tours_es", "message": "¡Bienvenido a la sección de TOURS!" }, ... ]

	}

	// Imprimir las plantillas guardadas en la consola
	// fmt.Println("Plantillas guardadas:")
	// for _, template := range messageTemplates {
	// 	fmt.Printf("ID: %s\nMessage: %s\n\n", template.ID, template.Message)
	// }

	// Ahora vamos a iniciar el servidor HTTP para recibir mensajes de WhatsApp
	// que funcionan como Webhooks, lo que significa que Facebook envía mensajes
	// a nuestro servidor cuando un usuario envía un mensaje a nuestro número de WhatsApp
	// y nosotros vamos a responder a esos mensajes según el flujo de la conversación
	// que hemos definido en la aplicación
	// estos mensajes llegan en un formato JSON que debemos decodificar y manejar
	// para enviar respuestas a los usuarios
	// Un ejemplo del webhook es el siguiente:
	// https://whatsapp.brote.org/webhook

	http.HandleFunc("/webhook", handleWebhook)
	http.HandleFunc("/enviar-mensaje", enviarMensajeSinPlantilla)

	// Iniciar el servidor HTTP en el puerto 9876

	// La siguiente linea funciona asi, si err es diferente de nil, entonces
	// se imprime el error y se termina la ejecución del programa

	if err := http.ListenAndServe(fmt.Sprintf(":%d", port), nil); err != nil {

		fmt.Printf("Error al iniciar el servidor: %s\n", err)
	}

	defer cerrarBaseDeDatos()

}

// Creamos la función handleWebhook que recibe dos parámetros
// w que es un objeto de tipo http.ResponseWriter y r que es un objeto de tipo http.Request
// w significaria la respuesta que vamos a enviar al cliente
// r es la solicitud que el cliente envia al servidor
// En esta función vamos a manejar los mensajes que llegan al servidor
// desde WhatsApp y vamos a enviar respuestas a los usuarios según el flujo de la conversación

// un ejemplo de w seria el siguiente:
// w.Header().Set("Content-Type", "application/json")
// w.WriteHeader(http.StatusOK)
// json.NewEncoder(w).Encode(map[string]interface{}{"status": "success", "message": "Mensaje recibido"})
// en este caso estamos enviando una respuesta en formato JSON con un status 200

// y de r seria el siguiente:
// r.Method
// r.Body
// r.URL.Query().Get("parametro") como por ejemplo ?parametro=valor
// r.Header.Get("Content-Type")

// los metodos HTTP son GET, POST, PUT, DELETE, PATCH, OPTIONS, HEAD
// en este caso solo vamos a manejar el metodo POST
// generalmente los mensajes de WhatsApp llegan en formato JSON
// y cuando navegamos usamos el metodo GET

func handleWebhook(w http.ResponseWriter, r *http.Request) {
	// Verificar que el método sea POST
	if r.Method == http.MethodPost {
		// Leer el cuerpo del mensaje
		body, err := ioutil.ReadAll(r.Body)
		if err != nil {
			// Si hay un error al leer el cuerpo del mensaje, enviar un error 500
			http.Error(w, "Error al leer el cuerpo del mensaje", http.StatusInternalServerError)
			return
		}

		// Imprimir el cuerpo del mensaje en la consola
		println(string(body))

		// Decodificar el cuerpo del mensaje en formato JSON
		var webhookData map[string]interface{}
		err = json.Unmarshal(body, &webhookData)
		if err != nil {
			http.Error(w, "Error al decodificar el JSON", http.StatusBadRequest)
			return
		}

		// Verificar que el cuerpo del mensaje tenga la estructura esperada
		entries, ok := webhookData["entry"].([]interface{})
		if !ok || len(entries) == 0 {
			http.Error(w, "Entrada de mensaje no válida", http.StatusBadRequest)
			return
		}

		// Iterar sobre las entradas

		for _, entry := range entries {
			// Verificar que la entrada tenga la estructura esperada
			entryMap, ok := entry.(map[string]interface{})
			if !ok {
				continue
			}

			// Verificar que la entrada tenga el campo "changes"
			changes, ok := entryMap["changes"].([]interface{})
			if !ok || len(changes) == 0 {
				continue
			}

			// Iterar sobre los cambios
			for _, change := range changes {
				// Verificar que el cambio tenga la estructura esperada
				changeMap, ok := change.(map[string]interface{})
				if !ok {
					continue
				}
				// Verificar que el cambio tenga el campo "value"
				value, ok := changeMap["value"].(map[string]interface{})
				if !ok {
					continue
				}

				// Verificar que el cambio tenga el campo "messages"
				messages, ok := value["messages"].([]interface{})
				if !ok || len(messages) == 0 {
					continue
				}

				// Iterar sobre los mensajes
				for _, message := range messages {
					// Verificar que el mensaje tenga la estructura esperada
					messageMap, ok := message.(map[string]interface{})
					if !ok {
						continue
					}
					// Verificar que el mensaje tenga el campo "from"
					from, ok := messageMap["from"].(string)
					if !ok {
						continue
					}
					// Verificar que el mensaje tenga el campo "text"

					text, ok := messageMap["text"].(map[string]interface{})
					if !ok {
						continue
					}

					// Verificar que el mensaje tenga el campo "body"
					body, ok := text["body"].(string)
					if !ok {
						continue
					}

					// Guardar el mensaje recibido en la base de datos
					err = guardarMensaje(from, "RECIBIDO", body)

					// un ejemplo de como nos llega el mensaje al webhook desde facebook seria
					// {
					// 	"entry": [
					// 		{
					// 			"changes": [
					// 				{
					// 					"value": {
					// 						"messages": [
					// 							{
					// 								"from": "5491123456789",
					// 								"text": {
					// 									"body": "Hola"
					// 								}
					// 							}
					// 						]
					// 					}
					// 				}
					// 			]
					// 		}
					// 	]
					// }

					if err != nil {
						fmt.Println("Error al guardar el mensaje recibido:", err)
						w.WriteHeader(http.StatusInternalServerError)
						return
					}

					// Imprimir el número del remitente y el contenido del mensaje en la consola

					fmt.Printf("Número del remitente: %s\n", from)
					fmt.Printf("Contenido del mensaje: %s\n", body)

					// Obtener el estado actual del usuario desde la base de datos
					estadoActual, _, err := obtenerEstadoUsuario(from)
					if err != nil {
						fmt.Println("Error al obtener el estado del usuario:", err)
						w.WriteHeader(http.StatusInternalServerError)
						return
					}
					println(estadoActual)
					// Si el usuario no tiene un estado almacenado, el estado actual es el estado principal
					if estadoActual == "" {
						estadoActual = estadoPrincipal
					}
					println(estadoActual)
					fmt.Printf("Usuario: %s\n", from)

					// Manejar el flujo según el estado actual
					switch estadoActual {
					case estadoPrincipal:
						// Lógica para el menú principal
						manejarOpcionMenuPrincipal(from, body)
					case estadoTours:
						// Lógica para la sección de TOURS
						manejarOpcionTours(from, body)

					case estadoTraslados:
						// Lógica para la sección de TOURS
						manejarOpcionTraslados(from, body)

						// case estadoAgente:
						// 	// Lógica para la sección de TOURS
						// 	//enviarMensaje(from, body)

					}
				}
			}
		}

		// Enviar una respuesta 200 OK
		w.WriteHeader(http.StatusOK)
	} else {
		// Enviar un error 405 Method Not Allowed
		http.Error(w, "Método no permitido", http.StatusMethodNotAllowed)
	}
}

// Esta función se encarga de manejar
// las opciones del menú principal

func manejarOpcionMenuPrincipal(numero, opcion string) {
	// Realizar acciones según la opción del menú principal
	// que es el estado actual del usuario
	// y según la opción que el usuario envía
	// vamos a enviar un mensaje al usuario según la opción que elija
	// y vamos a actualizar el estado del usuario en la base de datos
	// para que la próxima vez que el usuario envíe un mensaje
	// podamos manejar el flujo de la conversación según el estado actual
	// que el usuario tiene en la base de datos

	switch opcion {
	case "1":
		// Por ejemplo, si el usuario elige la opción 1, vamos a enviar un mensaje
		// con la plantilla "tours_es" y vamos a actualizar el estado del usuario
		// a "TOURS" en la base de datos

		err := actualizarEstadoUsuario(numero, estadoTours)
		if err != nil {
			fmt.Println("Error al actualizar el estado del usuario:", err)
			// Puedes manejar el error de la manera que consideres apropiada
		}

		println("Actualizamos estado")

		// Obtener el estado actual del usuario desde la base de datos

		estadoActual, _, err := obtenerEstadoUsuario(numero)
		if err != nil {
			fmt.Println("Error al obtener el estado del usuario:", err)
			return
		}

		// Imprimir el estado actual del usuario en la consola
		// es lo mismo usar fmt.println que fmt.Printf?

		println(estadoActual)
		fmt.Printf("Estado actualizado: %s\n", estadoTours)
		fmt.Printf("Usuario: %s\n", numero)

		// Una vez modificado enviamos el mensaje

		enviarMensaje(numero, "tours_es")

		// Si queremos podemos imprimir en la consola pero ahora lo vamos a deshabilitar
		// para que no se muestre en la consola
		// fmt.Println("tours_es")

	case "2":

		// La opción 1 en el menú principal ahora lleva a la sección de TOURS
		err := actualizarEstadoUsuario(numero, estadoTraslados)
		if err != nil {
			fmt.Println("Error al actualizar el estado del usuario:", err)
			// Puedes manejar el error de la manera que consideres apropiada
		}
		println("Actualizamos estado")
		// Obtener el estado actual del usuario desde la base de datos
		estadoActual, _, err := obtenerEstadoUsuario(numero)
		if err != nil {
			fmt.Println("Error al obtener el estado del usuario:", err)
			return
		}
		println(estadoActual)
		fmt.Printf("Estado actualizado: %s\n", estadoTraslados)
		fmt.Printf("Usuario: %s\n", numero)

		// Lógica para la opción 2 del menú principal
		enviarMensaje(numero, "transport_es")
		println("transport_es")

	case "3":
		// Lógica de 404 error
		enviarMensaje(numero, "404_es")

	case "4":
		// Lógica de 404 error
		enviarMensaje(numero, "404_es")

	case "5":
		// Lógica de 404 error
		enviarMensaje(numero, "404_es")

	case "6":
		// Lógica de 404 error
		enviarMensaje(numero, "404_es")

	case "agente":
		// Lógica de opciòn AGENTE
		enviarMensaje(numero, "agent_es")

	default:
		// Opción no reconocida en el menú principal

		err := actualizarEstadoUsuario(numero, estadoPrincipal)
		if err != nil {
			fmt.Println("Error al actualizar el estado del usuario:", err)
			// Puedes manejar el error de la manera que consideres apropiada
		}
		println("Actualizamos estado")
		// Opción no reconocida en el menú principal
		enviarMensaje(numero, "greeting_es")
		// println("greeting_es")
	}
}

func manejarOpcionTraslados(numero, opcion string) {
	// Realizar acciones según la opción de la sección de TOURS
	switch opcion {
	case "1":
		// Lógica para la opción 1 en la sección de TOURS
		enviarMensaje(numero, "404_es")
		// Puedes seguir agregando más casos según sea necesario

	case "2":
		// Lógica para la opción 1 en la sección de TOURS
		enviarMensaje(numero, "404_es")
		// Puedes seguir agregando más casos según sea necesario
	default:
		// Opción no reconocida en la sección de TOURS
		err := actualizarEstadoUsuario(numero, estadoPrincipal)
		if err != nil {
			fmt.Println("Error al actualizar el estado del usuario:", err)
			// Puedes manejar el error de la manera que consideres apropiada
		}
		println("Actualizamos estado")
		enviarMensaje(numero, "greeting_es")
	}
}

func manejarOpcionTours(numero, opcion string) {
	// Realizar acciones según la opción de la sección de TOURS
	switch opcion {
	case "1":
		// Lógica para la opción 1 en la sección de TOURS
		enviarMensaje(numero, "404_es")
		// Puedes seguir agregando más casos según sea necesario

	case "2":
		// Lógica para la opción 1 en la sección de TOURS
		enviarMensaje(numero, "404_es")
		// Puedes seguir agregando más casos según sea necesario
	default:
		// Opción no reconocida en la sección de TOURS
		err := actualizarEstadoUsuario(numero, estadoPrincipal)
		if err != nil {
			fmt.Println("Error al actualizar el estado del usuario:", err)
			// Puedes manejar el error de la manera que consideres apropiada
		}
		println("Actualizamos estado")
		enviarMensaje(numero, "greeting_es")
	}
}

// Esta función se encarga de obtener el estado actual del usuario desde la base de datos
// según el número de teléfono del usuario
// y devuelve el estado actual del usuario y un error si lo hay
// si el usuario no tiene un estado almacenado, devuelve una cadena vacía y un error nulo
// si hay un error al obtener el estado del usuario, devuelve una cadena vacía y un error no nulo

func obtenerEstadoUsuario(numero string) (string, string, error) {

	// Primero declaramos una variable estado de tipo string
	var estado string
	var fechaActualizacion string
	// luego ejecutamos la consulta a la base de datos
	err := db.QueryRow("SELECT estado, fecha_actualizacion FROM "+usuariosTabla+" WHERE numero = ?", numero).Scan(&estado, &fechaActualizacion)
	if err != nil {
		if err == sql.ErrNoRows {
			// Si no hay filas, el usuario no tiene un estado almacenado, devolvemos una cadena vacía
			return "", "", nil
		}
		return "", "", err
	}

	// verificar si la fecha de actualización es superior a 24 horas
	// si es superior a 24 horas, devolvemos el estado Menu Principal
	// si no es superior a 24 horas, devolvemos el estado actual del usuario

	// necesitamos convertir fechaActualizacion a un formato de fecha
	// para poder compararla con la fecha actual

	fechaActualizacionDate, err := time.Parse(time.RFC3339, fechaActualizacion)

	if err != nil {
		return "", "", err
	}

	if (fechaActualizacionDate.Add(time.Hour * 24)).Before(time.Now()) {
		estado = estadoPrincipal
	}

	// si el estado era Agente solo verificar si es menor a 4 horas sino lo mandamos al EstadoPrincipal
	if estado == estadoAgente {
		if (fechaActualizacionDate.Add(time.Hour * 4)).Before(time.Now()) {
			estado = estadoPrincipal
		}
	}

	// finalmente devolvemos el estado del usuario y un error nulo
	return estado, fechaActualizacion, nil
}

// Esta función se encarga de actualizar el estado del usuario en la base de datos
func actualizarEstadoUsuario(numero, estado string) error {

	// En go la variable _ se usa para ignorar el valor de retorno
	// en este caso, ignoramos el valor de retorno de la consulta
	// ya que no nos interesa en este caso
	// pero si te interesa, puedes usarlo
	// por ejemplo, si quieres saber cuántas filas se han actualizado
	// puedes usar el valor de retorno para eso

	// necesito que el estado tenga una expiración, entonces que se guarde en la base de datos el estado y la fecha de actualización

	fechaActualizacion := time.Now().Format("2006-01-02 15:04:05")

	// se guardará la fecha como un DATE en la base de datos

	_, err := db.Exec("INSERT OR REPLACE INTO "+usuariosTabla+" (numero, estado, fecha_actualizacion) VALUES (?, ?, ?)", numero, estado, fechaActualizacion)
	return err
}

// Esta función se encarga de guardar los mensajes en la base de datos
// para que podamos ver el historial de mensajes en la aplicación

func guardarMensaje(numero, tipo, mensaje string) error {
	// Obtenemos la fecha y hora actual en formato "YYYY-MM-DD HH:MM:SS"
	timestamp := time.Now().Format("2006-01-02 15:04:05")
	// Ejecutamos la consulta para guardar el mensaje en la base de datos
	_, err := db.Exec("INSERT INTO "+mensajesTabla+" (numero, tipo, mensaje, timestamp) VALUES (?, ?, ?, ?)", numero, tipo, mensaje, timestamp)
	return err
}

// Esta función se encarga de enviar mensajes a los usuarios
// según el contenido del mensaje que el usuario envía
// y según el estado actual del usuario
// por ejemplo, si el usuario envía un mensaje con la palabra "hola"
// vamos a enviar un mensaje de bienvenida al usuario

func enviarMensaje(numero, contenido string) {
	// Seleccionamos la plantilla en función del contenido del mensaje
	var templateName string

	// Si el contenido no está definido vamos a enviar greeting_es, caso contrario enviamos el contenido
	if contenido == "" {
		templateName = "greeting_es"
	} else {
		templateName = contenido
	}

	// Crear el cuerpo del mensaje en formato JSON
	// en este caso, solo necesitamos el número del destinatario y el nombre de la plantilla
	// pero puedes agregar más campos según sea necesario
	// por ejemplo, si quieres enviar un mensaje con un botón, puedes agregar el campo "buttons"
	// y si quieres enviar un mensaje con una imagen, puedes agregar el campo "image"
	// y así sucesivamente

	// el formato de este payload es JSON
	// y la estructura de ejemplo para un saludo es la siguiente:
	// {
	// 	"messaging_product": "whatsapp",
	// 	"to": "5491123456789",
	// 	"type": "template",
	// 	"template": {
	// 		"name": "greeting_es",
	// 		"language": {
	// 			"code": "es_AR"
	// 		}
	// 	}

	// si nosotros quisieramos enviar un mensaje con un pdf por ejemplo, el payload seria el siguiente:
	// {
	// 	"messaging_product": "whatsapp",
	// 	"to": "5491123456789",
	// 	"type": "document",
	// 	"document": {
	// 		"file": "https://example.com/document.pdf",
	// 		"filename": "document.pdf",
	// 		"caption": "This is a document"
	// 	}
	// }

	payload := []byte(fmt.Sprintf(`{
		"messaging_product": "whatsapp",
		"to": "%s",
		"type": "template",
		"template": {
			"name": "%s",
			"language": {
				"code": "es_AR"
			}
		}
	}`, numero, templateName))

	// ID de la plantilla que buscas
	targetID := templateName

	// Buscar la plantilla por ID
	// y obtener el mensaje que vamos a enviar al usuario
	// y guardarlo en la base de datos

	var targetMessage string
	println(targetID)
	for _, template := range messageTemplates {
		println(template.ID)
		if template.ID == targetID {
			targetMessage = template.Message
			break
		}
	}

	// Crear la solicitud HTTP POST
	// para enviar el mensaje al usuario

	req, err := http.NewRequest("POST", whatsappUrl, bytes.NewBuffer(payload))
	if err != nil {
		fmt.Println("Error al crear la solicitud HTTP:", err)
		return
	}

	// Agregar encabezados necesarios
	req.Header.Set("Authorization", "Bearer "+whatsappToken)
	req.Header.Set("Content-Type", "application/json")

	// Realizar la solicitud HTTP
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		fmt.Println("Error al realizar la solicitud HTTP:", err)
		return
	}
	defer resp.Body.Close()

	// ahora vamos a guardar el mensaje que vamos a enviar al usuario en la base de datos
	// para tener un registro de los mensajes enviados

	err = guardarMensaje(numero, "ENVIADO", targetMessage)

	if err != nil {
		fmt.Println("Error al guardar el mensaje recibido:", err)
		return
	}

	// Imprimir el código de estado de la respuesta
	fmt.Println("Código de estado:", resp.Status)
}

// Necesito crear una función para enviar un mensaje sin plantilla
// Esto seria para que cuando el agente vea la conversacion en el panel
// Pueda responderle el whatsapp desde el panel de gestión
// y no desde el celular

func enviarMensajeSinPlantilla(w http.ResponseWriter, r *http.Request) {
	// Verificar que el método sea POST
	// el formato que voy a enviar es {'numero': '5491123456789', 'contenido': 'Hola, ¿cómo estás?'}
	// definimos las variables que vamos a recibir

	// los mensajes sin plantilla solo se pueden enviar si el usuario inicio la conversación en las últimas 24 horas

	var contenido string

	if r.Method == http.MethodPost {

		// Leer el cuerpo del mensaje
		body, err := ioutil.ReadAll(r.Body)
		if err != nil {
			// Si hay un error al leer el cuerpo del mensaje, enviar un error 500
			http.Error(w, "Error al leer el cuerpo del mensaje", http.StatusInternalServerError)
			return
		}

		// Decodificar el cuerpo del mensaje en formato JSON
		var webhookData map[string]interface{}
		err = json.Unmarshal(body, &webhookData)
		if err != nil {
			http.Error(w, "Error al decodificar el JSON", http.StatusBadRequest)
			return
		}

		// Verificar que el cuerpo del mensaje tenga la estructura esperada
		// y que tenga los campos numero y contenido
		// si no tiene esos campos, devolvemos un error 400
		// que significa "solicitud incorrecta"
		// y si tiene esos campos, guardamos el valor en las variables correspondientes
		// y enviamos el mensaje al usuario

		numero, ok := webhookData["numero"].(string)
		if !ok {
			http.Error(w, "Número no válido", http.StatusBadRequest)
			return
		}

		contenido, ok = webhookData["contenido"].(string)
		if !ok {
			http.Error(w, "Contenido no válido", http.StatusBadRequest)
			return
		}

		// verificar si el numero está en la base de datos y si su ultimo estado fue dentro de las ultimas 24 horas
		// si no está en la base de datos o su ultimo estado fue hace mas de 24 horas, no se le puede enviar un mensaje sin plantilla

		_, fechaEstado, err := obtenerEstadoUsuario(numero)
		if err != nil {
			http.Error(w, "Error al obtener el estado del usuario", http.StatusInternalServerError)
			return
		}
		fechaEstadoDate, err := time.Parse(time.RFC3339, fechaEstado)
		if err != nil {
			http.Error(w, "Error al obtener la fecha de actualización del usuario", http.StatusInternalServerError)
			return
		}

		// si la fechaEstadoDate es superior a 24 horas, no se le puede enviar un mensaje sin plantilla
		if fechaEstadoDate.Add(time.Hour * 24).Before(time.Now()) {
			http.Error(w, "El usuario no ha iniciado la conversación en las últimas 24 horas", http.StatusBadRequest)
			return
		}

		// otra condicional es que si el contenido del mensaje es /cerrar, entonces el estado del usuario se actualiza a estadoPrincipal
		// y se le envia un mensaje de despedida

		if contenido == "/cerrar" {
			err := actualizarEstadoUsuario(numero, estadoPrincipal)
			if err != nil {
				http.Error(w, "Error al actualizar el estado del usuario", http.StatusInternalServerError)
				return
			}
			enviarMensaje(numero, "goodbye_es")
			return
		}

		// Crear el cuerpo del mensaje en formato JSON
		// en este caso, solo necesitamos el número del destinatario y el contenido del mensaje
		// pero puedes agregar más campos según sea necesario
		// por ejemplo, si quieres enviar un mensaje con un botón, puedes agregar el campo "buttons"
		// y si quieres enviar un mensaje con una imagen, puedes agregar el campo "image"
		// y así sucesivamente

		// el formato de este payload es JSON
		// y la estructura de ejemplo para un saludo es la siguiente:
		// {
		// 	"messaging_product": "whatsapp",
		// 	"to": "5491123456789",
		// 	"type": "text",
		// 	"text": "Hola, ¿cómo estás?"
		// }

		payload := []byte(fmt.Sprintf(`{
			"messaging_product": "whatsapp",
			"to": "%s",
			"type": "text",
			"text": {
				"body": "%s"
			}
		}`, numero, contenido))

		// Crear la solicitud HTTP POST
		// para enviar el mensaje al usuario

		req, err := http.NewRequest("POST", whatsappUrl, bytes.NewBuffer(payload))
		if err != nil {
			fmt.Println("Error al crear la solicitud HTTP:", err)
			return
		}

		// necesito imprimir el payload en la consola
		// para ver si el mensaje se envio correctamente
		// y si no se envio correctamente, ver el error

		fmt.Println(string(payload))
		fmt.Println(string(whatsappToken))
		fmt.Println(whatsappUrl)

		// Agregar encabezados necesarios
		req.Header.Set("Authorization", "Bearer "+whatsappToken)
		req.Header.Set("Content-Type", "application/json")

		// Realizar la solicitud HTTP
		client := &http.Client{}
		resp, err := client.Do(req)
		if err != nil {
			fmt.Println("Error al realizar la solicitud HTTP:", err)
			return
		}

		err = guardarMensaje(numero, "ENVIADO", contenido)

		if err != nil {
			fmt.Println("Error al guardar el mensaje recibido:", err)
			return
		}
		err = actualizarEstadoUsuario(numero, estadoAgente)
		if err != nil {
			fmt.Println("Error al actualizar el estado del usuario:", err)
			// Puedes manejar el error de la manera que consideres apropiada
		}

		defer resp.Body.Close()
	}
}

// creame el README.MD, es interno no es para que otro lo use

// el README.MD debe tener la siguiente estructura

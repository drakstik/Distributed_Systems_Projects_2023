package test.naming;

import java.io.IOException;
import java.net.InetSocketAddress;
import java.net.Socket;
import java.net.URI;
import java.net.http.HttpClient;
import java.net.http.HttpRequest;
import java.net.http.HttpResponse;
import java.util.Arrays;
import java.util.Enumeration;

import com.google.gson.Gson;
import test.ServerCommands;
import test.util.Test;
import test.util.TestFailed;
import common.PathRequest;

/** Base class of naming server tests.

    <p>
    On initialization, this class starts a naming server and creates infos for
    its service and registration interfaces. These objects are then accessible
    to subclasses of <code>NamingTest</code>.

    <p>
    Derived classes should override <code>testServer</code> to include the main
    test code. Derived classes should override <code>initialize</code> and
    <code>clean</code> if they have additional servers or system objects to
    create, start, stop, and clean up.
 */
abstract class NamingTest extends Test {
    /** Naming server under test. */
    private Process server = null;

    protected Gson gson;

    protected int service_port;
    protected int registration_port;

    protected HttpResponse<String> getResponse(String method, int port,
                                               Object requestObj) throws IOException, InterruptedException {

        HttpResponse<String> response;
        HttpRequest request = HttpRequest.newBuilder().uri(URI.create("http://localhost:" + port + method))
                .setHeader("Content-Type", "application/json")
                .POST(HttpRequest.BodyPublishers.ofString(gson.toJson(requestObj)))
                .build();
        try {
            PathRequest pathRequest = (PathRequest) requestObj;
            String path = pathRequest.getPath();
            // System.out.printf("Sending "+method+" request of path: "+path+".\n");
        } catch (ClassCastException e) {
            // Handle other types of request objects
            // System.out.println("WARNING: Request object not a path");
            // System.out.printf("Request "+method+".\n");
        }
        
        // Blocking call https://docs.oracle.com/en/java/javase/11/docs/api/java.net.http/java/net/http/HttpClient.html
        response = HttpClient.newHttpClient().send(request, HttpResponse.BodyHandlers.ofString());
        // System.out.printf("Received response: %s\n", response.toString());
        return response;
    }

    abstract protected void perform() throws TestFailed;

    /** Initializes the test.

        <p>
        This method starts the naming server and creates the infos through which
        its interfaces can be accessed.

        @throws TestFailed If the naming server cannot be started, or if the
                           infos cannot be created.
     */
    protected void initialize() throws TestFailed {
        gson = new Gson();
        Socket service_socket;
        Socket registration_socket;
        String[] splits = ServerCommands.namingCommand.split(" ");
        int n = splits.length;
        service_port = Integer.parseInt(splits[n - 2]);
        registration_port = Integer.parseInt(splits[n - 1]);

        try {
            ProcessBuilder b = new ProcessBuilder(Arrays.asList(splits));
            // b.inheritIO();
            // System.out.printf("Starting NamingServer...\n", Arrays.asList(splits).toString());
            server = b.start(); // "go run ../../naming/NamingServer.go 8080 8090"
        } catch (Throwable t) {
            throw new TestFailed("unable to start naming server");
        }

        // Attempt to make the connection.
        while (true) {
            try {
                service_socket = new Socket();
                service_socket.connect(new InetSocketAddress("127.0.0.1", service_port));
                registration_socket = new Socket();
                registration_socket.connect(new InetSocketAddress("127.0.0.1", registration_port));
                break;
            } catch (IOException e){
                // Ignore the exception to give server some time to start up
            }
        }

        // Make a best effort to close the socket if the connection is
        // successful.
        try {
            service_socket.close();
            registration_socket.close();
        } catch(IOException e) { }
    }

    /** Stops the naming server when the test completes.

        <p>
        If a subclass overrides this method, the new implementation should call
        this method before proceeding to do anything else.
     */
    @Override
    protected void clean() {
        if(server != null) {
            kill(server.toHandle());

            // Wait for the naming server to stop.
            try {
                server.waitFor();
            } catch(InterruptedException e) { }
            server = null;
        }
    }

    protected void kill(ProcessHandle handle) {
        handle.descendants().forEach(this::kill);
        handle.destroy();
    }
}

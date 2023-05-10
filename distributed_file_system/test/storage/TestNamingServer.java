package test.storage;

import java.io.IOException;
import java.io.InputStreamReader;
import java.io.OutputStream;
import java.net.InetSocketAddress;
import java.util.ArrayList;
import java.util.concurrent.Executors;

import common.FilesReturn;
import common.RegisterRequest;
import common.ServerInfo;

import com.google.gson.Gson;
import com.sun.net.httpserver.HttpServer;
import com.sun.net.httpserver.HttpExchange;
import test.common.Path;
import test.util.Test;
import test.util.TestFailed;
import test.util.TestUtil;

/** Test naming server.

    <p>
    This naming server performs the following checks each time a storage server
    registers:
    <ul>
    <li>The correct file list has been sent.</li>
    </ul>
 */
public class TestNamingServer {
    /** Test object that is running this server. */
    private final Test test;
    /** List of paths to expect from the next storage server to register. */
    private Path[] expect_files = null;
    /** List of paths to command the next storage server to register to
        delete. */
    private Path[] delete_files = null;
    /** Naming server IP address. */
    private final static String NAMING_IP = "127.0.0.1";
    /** Naming server registration port. */
    private int registration_port;
    /** Naming server registration interface service. */
    private HttpServer registration_service;
    /** Last registered storage server client interface. */
    private ServerInfo client_info = null;
    /** Last registered storage server command interface. */
    private ServerInfo command_info = null;
    /** Indicates that the service has started. */
    private boolean services_started = false;
    /** Gson object which can parse json to an object. */
    protected Gson gson;

    /** Creates the test naming server.

        @param test The test which is running this naming server.
     */
    TestNamingServer(Test test, int registration_port) throws IOException {
        this.test = test;
        this.registration_service = HttpServer.create(new InetSocketAddress(registration_port), 0);
        this.registration_service.setExecutor(Executors.newCachedThreadPool());
        this.registration_port = registration_port;
        this.gson = new Gson();
    }

    /** Sets the files the next storage server to connect is expected to
        register.

        @param files The files to expect. The naming server will check that
                     these are indeed the files that are received. If this
                     argument is <code>null</code>, the naming server will not
                     perform the check.
     */
    public void expectFiles(Path[] files) {
        expect_files = files;
    }

    /** Sets the files the next storage server to connect will be commanded to
        delete.

        @param files The files to be deleted. If this argument is
        <code>null</code>, the naming server will not command the storage server
        to delete any files.
     */
    public void deleteFiles(Path[] files) {
        delete_files = files;
    }

    /** Returns the client interface for the last storage server to register. */
    public ServerInfo clientInterface() {
        return client_info;
    }

    /** Returns the command interface for the last storage server to
        register. */
    public ServerInfo commandInterface() {
        return command_info;
    }

    /** Retrieves a registration info for the test server.

        @return The info.
     */
    ServerInfo info() {
        return new ServerInfo(NAMING_IP, registration_port);
    }

    /** Starts the test naming server.

        @throws TestFailed If the test cannot be started.
     */
    void start() throws TestFailed {
        this.startServices();
    }

    /** Stops the test naming server. */
    void stop() {
        this.registration_service.stop(0);
    }

    private void startServices() {
        // Prevent repeated starting of the services and re-creation of info.
        if (this.services_started) return;

        this.add_registration_api();
        this.registration_service.start();
        this.add_service_api();

        this.services_started = true;
    }

    private void add_registration_api() {
        this.register();
    }

    private void add_service_api() {

    }

    private void register() {
        this.registration_service.createContext("/register", (exchange -> {
            String respText = "";
            int returnCode = 200;
            if ("POST".equals(exchange.getRequestMethod())) {
                // parse request json
                RegisterRequest registerRequest = null;
                try {
                    InputStreamReader isr = new InputStreamReader(exchange.getRequestBody(), "utf-8");
                    registerRequest = gson.fromJson(isr, RegisterRequest.class);
                } catch (Exception e) {
                    respText = "Error during parse JSON object!\n";
                    returnCode = 400;
                    this.generateResponseAndClose(exchange, respText, returnCode);
                    test.failure(new TestFailed("In <register>: Error during parse JSON object!"));
                    return;
                }

                ArrayList<Path> storage_server_files = new ArrayList<Path>();
                for (String file : registerRequest.files) {
                    Path filePath = new Path(file);
                    storage_server_files.add(filePath);
                }

                // If expect_files is not null, make sure that the files list received
                // is the same as the files list expected.
                if (this.expect_files != null) {
                    if(!TestUtil.sameElements(storage_server_files.toArray(new Path[0]), expect_files)) {
                        test.failure(new TestFailed("received wrong file list during registration"));
                    }
                }

                // Set the info for the newly-registered server.
                ServerInfo client_info = new ServerInfo(registerRequest.storage_ip, registerRequest.client_port);
                this.client_info = client_info;
                
                ServerInfo command_info = new ServerInfo(registerRequest.storage_ip, registerRequest.command_port);
                this.command_info = command_info;

                // If delete_files is not null, return the list of files to delete.
                // Otherwise, the server is not to delete anything.
                ArrayList<String> to_be_deleted_files = new ArrayList<String>();
                if(delete_files != null) {
                    for (Path path : this.delete_files) {
                        to_be_deleted_files.add(path.toString());
                    }
                }

                FilesReturn filesReturn = new FilesReturn(to_be_deleted_files.toArray(new String[0]));
                respText = gson.toJson(filesReturn);
                returnCode = 200;
            } else {
                respText = "The REST method should be POST for <register>!\n";
                returnCode = 400;
                test.failure(new TestFailed("The REST method should be POST for <register>!"));
            }
            this.generateResponseAndClose(exchange, respText, returnCode);
        }));
    }

    /**
     * call this function when you want to write to response and close the connection.
     */
    private void generateResponseAndClose(HttpExchange exchange, String respText, int returnCode) throws IOException {
        exchange.sendResponseHeaders(returnCode, respText.getBytes().length);
        OutputStream output = exchange.getResponseBody();
        output.write(respText.getBytes());
        output.flush();
        exchange.close();
    }
}

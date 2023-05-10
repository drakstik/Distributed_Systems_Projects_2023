package test.storage;

import java.io.IOException;
import java.net.InetSocketAddress;
import java.net.Socket;
import java.net.URI;
import java.net.http.HttpClient;
import java.net.http.HttpRequest;
import java.net.http.HttpResponse;
import java.util.Arrays;

import com.google.gson.Gson;

import common.ServerInfo;
import test.ServerCommands;
import test.util.Test;
import test.util.TestFailed;

import test.common.Path;

/** Base class for storage server tests.

    <p>
    This class takes care of creating a temporary directory for the storage
    server to serve and starting a test naming server on startup, and removing
    the directory and stopping the servers on exit.
 */
abstract class StorageTest extends Test {
    /** Storage server being tested. */
    private Process server = null;

    /** Files to be created in the storage server root directory. */
    private String[][] test_files;
    /** Files that the naming server should command the storage server to delete
        after registration. */
    private Path[] delete_files;
    // root directory of the storage server.
    private String root_dir = null;
    /** Temporary directory served by the storage server. */
    protected TemporaryDirectory directory = null;
    /** Testing naming server. */
    protected TestNamingServer naming_server = null;
    /** Naming server registration service. */
    protected ServerInfo naming_info = null;
    protected Gson gson;

    /** Info for the storage server client service. */
    protected ServerInfo client_info = null;
    /** Info for the storage server command service. */
    protected ServerInfo command_info = null;


    /** Creates a <code>StorageTest</code> object.

        @param expect_files Files to be created in the storage server root
                          directory.
        @param delete_files Files the naming server is to command the storage
                            server to delete.
     */
    protected StorageTest(String[][] expect_files, Path[] delete_files) {
        this.test_files = expect_files;
        this.delete_files = delete_files;
        this.gson = new Gson();
    }

    /**
     * Initializes the temporary directory and servers to be used for the test.
     * 
     * @throws TestFailed           If the test objects cannot be initialized.
     * @throws InterruptedException
     */
    protected void initialize() throws TestFailed, IOException, InterruptedException {
        // get the root directory of the storage server
        int client_port, command_port;
        String[] splits = ServerCommands.storage0Command.split(" ");
        int n = splits.length;
        this.root_dir = splits[n - 1];

        // Create the first temporary directory.
        try {
            this.directory = new TemporaryDirectory(this.root_dir);
        } catch(Throwable t) {
            throw new TestFailed("unable to create first temporary directory " + this.root_dir, t);
        }
        // populate the root directory with expected files.
        try {
            if(this.test_files != null) {
                for(String[] path : test_files)
                    directory.add(path);
            }
        } catch(Throwable t) {
            throw new TestFailed("unable to populate first temporary directory", t);
        }

        // Assemble the list of expected files.
        Path[] expect_files = null;
        if (this.test_files != null) {
            expect_files = new Path[this.test_files.length];
            for(int index = 0; index < this.test_files.length; ++index) {
                String filePath = "";
                for (int i = 0; i < this.test_files[index].length; i++) {
                    filePath += "/" + this.test_files[index][i];
                }
                expect_files[index] = new Path(filePath);
            }
        }
        client_port = Integer.parseInt(splits[n - 4]);
        command_port = Integer.parseInt(splits[n - 3]);
        int registration_port = Integer.parseInt(splits[n - 2]);

        // first start the test naming server, which is a very simple naming server
        this.naming_server = new TestNamingServer(this, registration_port);
        this.naming_server.start();
        this.naming_server.expectFiles(expect_files);
        this.naming_server.deleteFiles(delete_files);
        this.naming_info = naming_server.info();

        // start a storage server according to the command line specified in ServerCommands.java
        Socket storage_socket;
        Socket registration_socket;

        try {
            ProcessBuilder b = new ProcessBuilder(Arrays.asList(splits));
            server = b.start();
        } catch (Throwable t) {
            throw new TestFailed("unable to start first storage server");
        }

        // Attempt to make the connection.
        while (true) {
            try {
                storage_socket = new Socket();
                storage_socket.connect(new InetSocketAddress("127.0.0.1", client_port));
                registration_socket = new Socket();
                registration_socket.connect(new InetSocketAddress("127.0.0.1", command_port));
                break;
            } catch (IOException e) {
                // Ignore the exception to give server some time to start up
            }
        }

        // Make a best effort to close the socket if the connection is successful.
        try {
            storage_socket.close();
            registration_socket.close();
        } catch(IOException e) { 
            e.printStackTrace();
        }

        // Retrieve the storage server infos.
        this.client_info = naming_server.clientInterface();
        this.command_info = naming_server.commandInterface();
    }

    /** Stops the testing servers and removes the temporary directory. */
    @Override
    protected void clean() {
        if (server != null) {
            kill(server.toHandle());
            // Wait for the storage server to stop.
            try {
                server.waitFor();
            } catch (InterruptedException e) { }
            server = null;
        }

        if (naming_server != null) {
            naming_server.stop();
            naming_server = null;
        }

        if (directory != null) {
            directory.remove();
            directory = null;
        }
    }

    protected HttpResponse<String> getResponse(String method, int port,
                                               Object requestObj) throws IOException, InterruptedException {

        HttpResponse<String> response;
        HttpRequest request = HttpRequest.newBuilder().uri(URI.create("http://localhost:" + port + method))
                .setHeader("Content-Type", "application/json")
                .POST(HttpRequest.BodyPublishers.ofString(gson.toJson(requestObj)))
                .build();

        response = HttpClient.newHttpClient().send(request, HttpResponse.BodyHandlers.ofString());
        return response;
    }

    protected void kill(ProcessHandle handle) {
        handle.descendants().forEach(this::kill);
        handle.destroy();
    }
}

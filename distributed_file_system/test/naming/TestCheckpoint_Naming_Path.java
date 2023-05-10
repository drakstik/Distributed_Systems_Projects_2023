package test.naming;

import common.BooleanReturn;
import common.ExceptionReturn;
import common.PathRequest;
import test.util.TestFailed;
import java.io.*;

import java.net.http.HttpResponse;

/** Tests the path library.

 <p>
 Tests include:
 <ul>
 <li>The constructors reject empty paths, components, and component strings
 containing the path separator character.</li>
 </ul>
 */
public class TestCheckpoint_Naming_Path extends NamingTest {
    /** Test notice. */
    public static final String notice = "checking path library public interface";
    

    /** Performs the tests.
     @throws TestFailed If any of the tests fail.
     */
    @Override
    protected void perform() throws TestFailed {
        testConstructors();
    }

    /** Tests <code>Path</code> constructors and the <code>toString</code> and
     <code>equals</code> methods.

     @throws TestFailed If any of the tests fail.
     */
    private void testConstructors() throws TestFailed {
        
        boolean result;
        String exception_type;

                
        // Make sure the naming server rejects strings that do not
        // begin with the separator or contain a colon.
        try {
            PathRequest request = new PathRequest(""); // Creating an empty path request
            HttpResponse<String> response = getResponse("/is_valid_path", service_port, request);
            exception_type = gson.fromJson(response.body(), ExceptionReturn.class).exception_type;
            if(exception_type != null) {
                throw new Throwable();
            }

            result = gson.fromJson(response.body(), BooleanReturn.class).success;
            if(result) {
                throw new TestFailed("Path constructor accepted empty string");
            }
        } catch(TestFailed e) { 
            throw e; 
        } catch(Throwable t) {
            throw new TestFailed("Path constructor threw unexpected exception " +
                                 "when given empty string", t);
        }

        try {
            PathRequest request = new PathRequest("some-file");
            HttpResponse<String> response = getResponse("/is_valid_path", service_port, request);
            exception_type = gson.fromJson(response.body(), ExceptionReturn.class).exception_type;
            if(exception_type != null) {
                throw new Throwable();
            }

            result = gson.fromJson(response.body(), BooleanReturn.class).success;
            if(result) {
                throw new TestFailed("Path constructor accepted string " +
                        "without required start delimiter");
            }
        } catch(TestFailed e) { 
            throw e; 
        } catch(Throwable t) {
            throw new TestFailed("Path constructor threw unexpected exception " +
                                 "when given string without required start delimiter", t);
        }

        try {
            PathRequest request = new PathRequest("hostname:path");
            HttpResponse<String> response = getResponse("/is_valid_path", service_port, request);
            exception_type = gson.fromJson(response.body(), ExceptionReturn.class).exception_type;
            if(exception_type != null) {
                throw new Throwable();
            }

            result = gson.fromJson(response.body(), BooleanReturn.class).success;
            if(result) {
                throw new TestFailed("Path constructor accepted string containing colon");
            }
        } catch(TestFailed e) { 
            throw e; 
        } catch(Throwable t) {
            throw new TestFailed("Path constructor threw unexpected exception " +
                                 "when given string containing colon", t);
        }
    }
}

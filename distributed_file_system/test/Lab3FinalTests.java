package test;

import test.util.Series;
import test.util.SeriesReport;
import test.util.Test;
import java.util.HashMap;
import java.util.Map;

public class Lab3FinalTests {
    /** Runs the tests.

     @param arguments Ignored.
     */
    public static void main(String[] arguments) {
        // Create the test list, the series object, and run the test series.
        @SuppressWarnings("unchecked")
        Class<? extends Test>[] tests = new Class[] {
            test.naming.TestCheckpoint_Naming_Path.class,
            test.naming.TestCheckpoint_Naming_Registration.class,
            test.naming.TestCheckpoint_Naming_Listing.class,
            test.naming.TestCheckpoint_Naming_Creation.class,
            test.naming.TestCheckpoint_Naming_InfoRetrieval.class,
            test.storage.TestCheckpoint_Storage_Registration.class,
            test.storage.TestCheckpoint_Storage_Access.class,
            test.storage.TestCheckpoint_Storage_Directory.class,
            test.naming.TestFinal_Naming_Lock.class,
            test.naming.TestFinal_Naming_Queue.class,
            test.naming.TestFinal_Naming_Replication.class,
            test.naming.TestFinal_Naming_Deletion.class,
            test.storage.TestFinal_Storage_Replication.class
        };
                
        Map<String, Integer> points = new HashMap<>();
        
        points.put("test.naming.TestCheckpoint_Naming_Path", 10);
        points.put("test.naming.TestCheckpoint_Naming_Registration", 10);
        points.put("test.naming.TestCheckpoint_Naming_Listing", 10);
        points.put("test.naming.TestCheckpoint_Naming_Creation", 10);
        points.put("test.naming.TestCheckpoint_Naming_InfoRetrieval", 10);
        points.put("test.storage.TestCheckpoint_Storage_Registration", 10);
        points.put("test.storage.TestCheckpoint_Storage_Access", 20);
        points.put("test.storage.TestCheckpoint_Storage_Directory", 20);
        points.put("test.naming.TestFinal_Naming_Lock", 20);
        points.put("test.naming.TestFinal_Naming_Queue", 25);
        points.put("test.naming.TestFinal_Naming_Replication", 25);
        points.put("test.naming.TestFinal_Naming_Deletion", 25);
        points.put("test.storage.TestFinal_Storage_Replication", 25);
        
        int runsOfEachTest = 1;
        
        Series series = new Series(tests, runsOfEachTest);
        SeriesReport report = series.run(10, System.out);

        // Print the report and exit with an appropriate exit status.
        report.print(System.out, points, runsOfEachTest);
        System.exit(report.successful() ? 0 : 2);
    }
}


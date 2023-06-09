package test.naming;

import java.net.http.HttpResponse;

import common.LockRequest;
import test.common.Path;
import test.util.TestFailed;

/** Tests lock queueing.

    <p>
    This test starts many threads. The first two attempt to lock the root
    directory for shared access. The third attempts to lock it for exclusive
    access. The next group of threads attempt to take it for shared access. The 
    next again attempts to lock for exclusive access. The last two attempt to
    lock it for shared access.  It then checks the following conditions:
    <ul>
    <li>The first two, middle group, and last two threads are able to take the lock
        together in three separate groups.</li>
    <li>The exclusive access threads cannot take the lock until after each group of
        shared access threads is done.</li>
    <li>The exclusive access threads take the lock before the subsequent groups of
        shared access threads.</li>
    </ul>

    <p>
    Note that each thread is started and allowed to try to take the lock before
    the next thread, but no thread releases the lock until all threads have been
    started. This allows all the threads to queue in the <code>lock</code>
    method before the rest of the test precedes.

    <p>
    Threads should be served from the queue in the order they arrived. Shared
    access threads arriving after an exclusive access thread is queued should
    not be able to take the lock ahead of the shared access user.
 */
public class TestFinal_Naming_Queue extends NamingTest {
    /** Test notice. */
    public static final String notice =
        "checking naming server lock queue fairness";
    /** Prerequisites. */
    public static final Class[] prerequisites = new Class[] {TestFinal_Naming_Lock.class};

    /** Path to the root directory. */
    private final Path root = new Path("/");
    
    /** Number of threads being created in the test. */
    private int num_threads = 16;
    /** Indicates that all threads have been started and have most likely queued
        at the lock. */
    private boolean all_threads_started = false;
    /** Number of threads that have succesfully locked the root directory. Each
        thread expects this not to exceed some other number. */
    private int lock_count = 0;
    /** Used to determine whether the current shared access user is the first at
        the rendezvous, and therefore should wait, or is the second, and
        therefore should wake the other thread. */
    private boolean rendezvous_first = true;
    /** Number of threads that have released the lock on the root directory.
        Used to determine whether the test is complete. */
    private int thread_exits = 0;

    /** Delay between thread starts, in milliseconds. This is long enough to
        make it very likely that each new thread will call <code>lock</code> and
        enter the lock's queue before the next thread started does so. */
    private static final int DELAY = 250;

    /** Indicates that the test is complete and any sleeping threads should be
        awakened. */
    private boolean wake_all = false;

    /** Performs the test. */
    @Override
    protected void perform() {
        // Start two shared access threads. These do not require any thread to
        // have taken the lock before them.
        startThread(false, 0);
        startThread(false, 0);

        // Start the exclusive access thread. This thread expects at least two
        // threads to have taken the lock before it - the two shared access
        // threads already started.
        startThread(true, 2);

        // Start several more shared access threads. These each expect at least
        // three threads to have taken the lock before them - the two shared
        // access threads started first, and the exclusive access thread started
        // after.
        for(int i=0; i<num_threads-6; i++)
            startThread(false, 3);

        // Start another exclusive access thread. This thread expects all previous
        // threads to have all taken the lock before it, including the first
        // exclusive thread.
        startThread(true, num_threads-3);

        // Start two more shared access threads. These each expect all previous
        // threads to have taken the lock before them, including all of the shared
        // access threads and the two exclusive threads.
        startThread(false, num_threads-2);
        startThread(false, num_threads-2);

        synchronized(this) {
            // Wake any threads and permit them to continue.
            all_threads_started = true;
            notifyAll();

            // Wait until all threads have exited, or the test terminated by
            // timeout.
            while(thread_exits < num_threads && !wake_all) {
                try {
                    wait();
                } catch(InterruptedException e) { }
            }
        }
    }

    /** Starts a thread and gives it time to enter the root directory lock's
        queue.

        @param exclusive Whether or not the thread is to request exclusive
                         access to the root directory.
        @param expect_lock_count Number of threads the new thread expects to
                                 have taken the lock by the time it does so.
     */
    private void startThread(boolean exclusive, int expect_lock_count) {
        // Start the new thread.
        new Thread(new QueuedLockUser(exclusive, expect_lock_count)).start();

        // Delay DELAY milliseconds.
        long current_time = System.currentTimeMillis();
        long wake_time = current_time + DELAY;

        while(current_time < wake_time && !wake_all) {
            try {
                Thread.sleep(wake_time - current_time);
            } catch(InterruptedException e) { }

            current_time = System.currentTimeMillis();
        }
    }

    /** Wakes all threads. */
    @Override
    protected void clean() {
        super.clean();

        synchronized(this) {
            wake_all = true;
            notifyAll();
        }
    }

    /** Queueing thread.

        <p>
        Each queueing thread attempts to take the lock on the root directory for
        either shared or exclusive access. After the thread has done so, it
        checks that no less than the expected number of threads have taken the
        lock before it. This condition will not be met if, for instance, shared
        access threads are able to cut ahead of exclusive access threads in the
        queue.

        <p>
        Furthermore, if the thread is taking the lock for shared access, it must
        rendezvous with another thread taking the lock for shared access
        simultaneously.
     */
    private class QueuedLockUser implements Runnable {
        /** Whether the thread is to take the lock for exclusive or shared
            access. */
        private final boolean exclusive;
        /** Minimum number of threads that must have taken the lock before this
            one. */
        private final int expect_lock_count;

        /** Creates the <code>QueuedLockUser</code> and sets its fields. */
        protected QueuedLockUser(boolean exclusive, int expect_lock_count) {
            this.exclusive = exclusive;
            this.expect_lock_count = expect_lock_count;
        }

        /** Runs the thread. */
        @Override
        public void run() {
            // Lock the root directory for the requested kind of access.
            try {
                LockRequest request = new LockRequest(root.toString(), exclusive);
                HttpResponse<String> response = getResponse("/lock", service_port, request);
                if(!response.body().isEmpty()) {
                    throw new Throwable(response.body());
                }
            } catch(Throwable t) {
                failure(new TestFailed("unable to lock root", t));
                return;
            }

            synchronized(TestFinal_Naming_Queue.this) {
                // Wait until all threads have been started. The first two
                // threads will be stopped here. If the implementation of locks
                // is correct, the third thread, which is requesting exclusive
                // access, will block in the lock method, as will all subsequent
                // threads. Therefore, by the time it reaches this code,
                // all_threads_started will already have been set to true.
                while(!all_threads_started && !wake_all) {
                    try {
                        TestFinal_Naming_Queue.this.wait();
                    } catch(InterruptedException e) { }
                }

                // Check that the expected number of threads have taken the lock
                // before this one.
                if(expect_lock_count > lock_count) {
                    failure(new TestFailed("thread took lock out of order: " +
                                           "expected at least " +
                                           expect_lock_count + " prior " +
                                           "lock(s), but found " + lock_count));
                    return;
                }

                // Increment the lock count to account for the current thread.
                ++lock_count;

                // If this thread is requesting shared access, rendezvous with a
                // paired thread requesting the same.
                if(!exclusive) {
                    // Toggle the rendezvous_first flag to indicate to the next
                    // thread that it should wake this thread.
                    rendezvous_first = !rendezvous_first;

                    // Until the second thread toggles the flag back, wait. If
                    // this is the second thread, wake the first thread.
                    if(!rendezvous_first) {
                        while(!rendezvous_first && !wake_all) {
                            try {
                                TestFinal_Naming_Queue.this.wait();
                            } catch(InterruptedException e) { }
                        }
                    } else
                        TestFinal_Naming_Queue.this.notifyAll();
                }
            }

            // Unlock the root directory.
            try {
                LockRequest request = new LockRequest(root.toString(), exclusive);
                HttpResponse<String> response = getResponse("/unlock", service_port, request);
                if(!response.body().isEmpty()) {
                    throw new Throwable(response.body());
                }
            } catch(Throwable t) {
                failure(new TestFailed("unable to unlock root", t));
                return;
            }

            // Increment the number of thread exits, and wake the main thread if
            // the number has reached five.
            synchronized(TestFinal_Naming_Queue.this) {
                ++thread_exits;
                TestFinal_Naming_Queue.this.notifyAll();
            }
        }
    }
}

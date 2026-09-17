package e2e;

import io.grpc.Status;
import io.littlehorse.sdk.common.proto.Comparator;
import io.littlehorse.sdk.common.proto.LHStatus;
import io.littlehorse.sdk.wfsdk.WfRunVariable;
import io.littlehorse.sdk.wfsdk.Workflow;
import io.littlehorse.test.LHTest;
import io.littlehorse.test.LHWorkflow;
import io.littlehorse.test.WorkflowVerifier;
import org.junit.jupiter.api.Assertions;
import org.junit.jupiter.api.Test;

/**
 * Tests the PutVariable RPC, which allows modifying the value of a Variable in a running WfRun.
 *
 * <p>The WfSpec used here blocks on a WAIT_FOR_CONDITION node, which lets us assert that mutating
 * a Variable from outside the WfRun causes anything depending on that Variable to be re-evaluated.
 */
@LHTest
public class PutVariableTest {

    private WorkflowVerifier verifier;

    @LHWorkflow("put-variable")
    public Workflow putVariableWorkflow;

    @Test
    void shouldUnblockWaitForConditionWhenVariableIsModified() {
        verifier.prepareRun(putVariableWorkflow)
                .thenVerifyWfRun(wfRun -> Assertions.assertEquals(LHStatus.RUNNING, wfRun.getStatus()))
                .thenPutVariable(0, "approved", true)
                .thenVerifyVariable(0, "approved", value -> Assertions.assertTrue(value.getBool()))
                .waitForStatus(LHStatus.COMPLETED)
                .start();
    }

    @Test
    void shouldModifyVariableThatNothingIsWaitingOn() {
        verifier.prepareRun(putVariableWorkflow)
                .thenPutVariable(0, "note", "modified-from-outside")
                .thenVerifyVariable(
                        0, "note", value -> Assertions.assertEquals("modified-from-outside", value.getStr()))
                .thenVerifyWfRun(wfRun -> Assertions.assertEquals(LHStatus.RUNNING, wfRun.getStatus()))
                .thenPutVariable(0, "approved", true)
                .waitForStatus(LHStatus.COMPLETED)
                .start();
    }

    @Test
    void shouldCreateVariableThatWfSpecDoesNotDeclare() {
        verifier.prepareRun(putVariableWorkflow)
                .thenPutVariable(0, "ad-hoc-variable", "created-by-put-variable")
                .thenVerifyVariable(
                        0,
                        "ad-hoc-variable",
                        value -> Assertions.assertEquals("created-by-put-variable", value.getStr()))
                .thenPutVariable(0, "approved", true)
                .waitForStatus(LHStatus.COMPLETED)
                .start();
    }

    @Test
    void shouldThrowInvalidArgumentWhenNewValueHasWrongType() {
        verifier.prepareRun(putVariableWorkflow)
                // 'approved' is a BOOL, so an INT is not a legal value for it.
                .thenPutVariable(
                        0,
                        "approved",
                        42,
                        exn -> Assertions.assertEquals(
                                Status.Code.INVALID_ARGUMENT, exn.getStatus().getCode()))
                .thenVerifyVariable(0, "approved", value -> Assertions.assertFalse(value.getBool()))
                .thenVerifyWfRun(wfRun -> Assertions.assertEquals(LHStatus.RUNNING, wfRun.getStatus()))
                .thenPutVariable(0, "approved", true)
                .waitForStatus(LHStatus.COMPLETED)
                .start();
    }

    @Test
    void shouldThrowNotFoundForUnknownThreadRun() {
        verifier.prepareRun(putVariableWorkflow)
                .thenPutVariable(
                        1234,
                        "approved",
                        true,
                        exn -> Assertions.assertEquals(
                                Status.Code.NOT_FOUND, exn.getStatus().getCode()))
                .thenVerifyWfRun(wfRun -> Assertions.assertEquals(LHStatus.RUNNING, wfRun.getStatus()))
                .thenPutVariable(0, "approved", true)
                .waitForStatus(LHStatus.COMPLETED)
                .start();
    }

    @LHWorkflow("put-variable")
    public Workflow getPutVariableWorkflow() {
        return Workflow.newWorkflow("put-variable", wf -> {
            WfRunVariable approved = wf.addVariable("approved", false);
            wf.addVariable("note", "unset");

            wf.waitForCondition(wf.condition(approved, Comparator.EQUALS, true));
        });
    }
}

package io.littlehorse.test.internal.step;

import io.grpc.StatusRuntimeException;
import io.littlehorse.sdk.common.proto.LittleHorseGrpc.LittleHorseBlockingStub;
import io.littlehorse.sdk.common.proto.PutVariableRequest;
import io.littlehorse.sdk.common.proto.VariableId;
import io.littlehorse.sdk.common.proto.VariableValue;
import io.littlehorse.test.internal.TestExecutionContext;
import java.util.function.Consumer;

public class PutVariableStep extends AbstractStep {

    private final int threadRunNumber;
    private final String variableName;
    private final VariableValue newValue;
    private final Consumer<StatusRuntimeException> exceptionConsumer;

    public PutVariableStep(
            int threadRunNumber,
            String variableName,
            VariableValue newValue,
            Consumer<StatusRuntimeException> exceptionConsumer,
            int id) {
        super(id);
        this.threadRunNumber = threadRunNumber;
        this.variableName = variableName;
        this.newValue = newValue;
        this.exceptionConsumer = exceptionConsumer;
    }

    @Override
    public void tryExecute(TestExecutionContext context, LittleHorseBlockingStub client) {
        try {
            client.putVariable(PutVariableRequest.newBuilder()
                    .setId(VariableId.newBuilder()
                            .setWfRunId(context.getWfRunId())
                            .setThreadRunNumber(threadRunNumber)
                            .setName(variableName)
                            .build())
                    .setValue(newValue)
                    .build());
        } catch (StatusRuntimeException exn) {
            if (exceptionConsumer != null) {
                exceptionConsumer.accept(exn);
            } else {
                throw exn;
            }
        }
    }
}

package io.littlehorse.common.model.corecommand.subcommand;

import com.google.protobuf.Message;
import io.grpc.Status;
import io.littlehorse.common.LHConstants;
import io.littlehorse.common.LHSerializable;
import io.littlehorse.common.LHServerConfig;
import io.littlehorse.common.exceptions.LHApiException;
import io.littlehorse.common.exceptions.LHValidationException;
import io.littlehorse.common.model.corecommand.CoreSubCommand;
import io.littlehorse.common.model.getable.core.variable.VariableModel;
import io.littlehorse.common.model.getable.core.variable.VariableValueModel;
import io.littlehorse.common.model.getable.core.wfrun.ThreadRunModel;
import io.littlehorse.common.model.getable.core.wfrun.WfRunModel;
import io.littlehorse.common.model.getable.global.wfspec.thread.ThreadVarDefModel;
import io.littlehorse.common.model.getable.global.wfspec.variable.VariableDefModel;
import io.littlehorse.common.model.getable.objectId.VariableIdModel;
import io.littlehorse.sdk.common.proto.PutVariableRequest;
import io.littlehorse.sdk.common.proto.Variable;
import io.littlehorse.sdk.common.proto.WfRunVariableAccessLevel;
import io.littlehorse.server.streams.storeinternals.GetableManager;
import io.littlehorse.server.streams.topology.core.CoreProcessorContext;
import io.littlehorse.server.streams.topology.core.ExecutionContext;
import lombok.Getter;
import lombok.Setter;

/**
 * Modifies the value of a single Variable in a WfRun.
 *
 * <p>The Variable is looked up on the ThreadRun specified by the request. If that ThreadRun does
 * not declare the Variable, we walk up the ThreadRun's parents until we find the ThreadRun which
 * owns it, which mirrors the resolution rules used when a WfSpec mutates a variable.
 *
 * <p>Once the new value has been written, we advance the WfRun so that anything which depends on
 * the Variable (most notably a {@code WAIT_FOR_CONDITION} node) is re-evaluated as part of this
 * same Command.
 */
@Getter
@Setter
public class PutVariableRequestModel extends CoreSubCommand<PutVariableRequest> {

    private VariableIdModel id;
    private VariableValueModel value;

    public PutVariableRequestModel() {}

    public PutVariableRequestModel(VariableIdModel id, VariableValueModel value) {
        this.id = id;
        this.value = value;
    }

    @Override
    public Class<PutVariableRequest> getProtoBaseClass() {
        return PutVariableRequest.class;
    }

    @Override
    public PutVariableRequest.Builder toProto() {
        PutVariableRequest.Builder out = PutVariableRequest.newBuilder().setId(id.toProto());
        if (value != null) {
            out.setValue(value.toProto());
        }
        return out;
    }

    @Override
    public void initFrom(Message proto, ExecutionContext ctx) {
        PutVariableRequest p = (PutVariableRequest) proto;
        this.id = LHSerializable.fromProto(p.getId(), VariableIdModel.class, ctx);
        this.value = p.hasValue() ? VariableValueModel.fromProto(p.getValue(), ctx) : new VariableValueModel();
    }

    @Override
    public String getPartitionKey() {
        return id.getPartitionKey().get();
    }

    @Override
    public Variable process(CoreProcessorContext ctx, LHServerConfig config) {
        GetableManager getableManager = ctx.getableManager();

        WfRunModel wfRun = getableManager.get(id.getWfRunId());
        if (wfRun == null) {
            throw new LHApiException(Status.NOT_FOUND, "Couldn't find WfRun %s".formatted(id.getWfRunId()));
        }

        ThreadRunModel specifiedThread = wfRun.getThreadRun(id.getThreadRunNumber());
        if (specifiedThread == null) {
            throw new LHApiException(
                    Status.NOT_FOUND,
                    "Couldn't find ThreadRun %d on WfRun %s".formatted(id.getThreadRunNumber(), id.getWfRunId()));
        }

        // Find the ThreadRun which actually owns the Variable, exactly as a variable mutation
        // from within the WfRun would.
        ThreadRunModel owningThread = specifiedThread;
        ThreadVarDefModel threadVarDef = null;
        while (owningThread != null) {
            ThreadVarDefModel localVarDef = owningThread.getThreadSpec().localGetVarDef(id.getName());
            if (localVarDef != null) {
                threadVarDef = localVarDef;
                break;
            }
            owningThread = owningThread.getParent();
        }

        int owningThreadRunNumber = owningThread != null ? owningThread.getNumber() : id.getThreadRunNumber();

        VariableValueModel newValue = this.value != null ? this.value : new VariableValueModel();
        Boolean masked = null;

        if (threadVarDef != null) {
            if (threadVarDef.getAccessLevel() == WfRunVariableAccessLevel.INHERITED_VAR) {
                throw new LHApiException(
                        Status.FAILED_PRECONDITION,
                        ("Variable %s is an INHERITED_VAR on WfRun %s; it must be modified on the parent"
                                        + " WfRun which owns it.")
                                .formatted(id.getName(), id.getWfRunId()));
            }

            VariableDefModel varDef = threadVarDef.getVarDef();
            masked = varDef.isMaskedValue();

            if (!newValue.isNull()) {
                try {
                    varDef.validateValue(newValue, ctx.metadataManager());
                    newValue = varDef.getTypeDef().applyCast(newValue);
                } catch (LHValidationException exn) {
                    throw new LHApiException(Status.INVALID_ARGUMENT, exn.getMessage());
                } catch (IllegalArgumentException exn) {
                    throw new LHApiException(
                            Status.INVALID_ARGUMENT,
                            "Provided value is not compatible with the declared type of Variable %s: %s"
                                    .formatted(id.getName(), exn.getMessage()));
                }
            }
        }

        VariableIdModel targetId = new VariableIdModel(id.getWfRunId(), owningThreadRunNumber, id.getName());
        VariableModel variable = getableManager.get(targetId);

        if (variable == null) {
            // The Variable doesn't exist yet: either it was never assigned, or the WfSpec doesn't
            // declare it at all. Either way we create it on the ThreadRun that owns it.
            variable = new VariableModel(
                    id.getName(),
                    newValue,
                    id.getWfRunId(),
                    owningThreadRunNumber,
                    wfRun.getWfSpec(),
                    masked != null && masked);
        } else {
            variable.setValue(newValue);
            if (masked != null) {
                variable.setMasked(masked);
            }
        }
        getableManager.put(variable);

        // Advance the WfRun so that anything blocked on this Variable (eg. a WAIT_FOR_CONDITION
        // node) reacts to the new value right away.
        wfRun.advance(ctx.currentCommand().getTime());

        // Advancing may have mutated the Variable again (for example via a VariableMutation on a
        // Node that was unblocked), so report whatever the WfRun ended up with.
        VariableModel afterAdvance = getableManager.get(targetId);
        if (afterAdvance != null) {
            variable = afterAdvance;
        }

        Variable.Builder out = variable.toProto();
        if (variable.isMasked()) {
            out.setValue(new VariableValueModel(LHConstants.STRING_MASK).toProto());
        }
        return out.build();
    }
}

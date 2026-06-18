package app

import (
	"fmt"
	"io"
)

type planShowJSONResponse struct {
	Plan *contractPlan `json:"plan"`
}

func (a App) PlanShow(stdout io.Writer, runID string, jsonOutput bool) error {
	context, ok, err := a.loadContractContext(stdout, runID, jsonOutput)
	if err != nil || !ok {
		return err
	}
	if jsonOutput {
		return writeJSONResponse(stdout, planShowJSONResponse{Plan: context.Contract.Plan})
	}
	writePlanShow(stdout, runID, context.State.Status, context.Contract.Plan)
	return nil
}

func writePlanShow(stdout io.Writer, runID string, runStatus string, plan *contractPlan) {
	fmt.Fprintln(stdout, "Plan")
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Run:")
	fmt.Fprintf(stdout, "  id: %s\n", runID)
	fmt.Fprintf(stdout, "  status: %s\n", runStatus)
	fmt.Fprintln(stdout)
	if plan == nil || len(plan.Tasks) == 0 {
		fmt.Fprintln(stdout, "No plan defined for this contract.")
		return
	}
	writePlanSection(stdout, plan)
}

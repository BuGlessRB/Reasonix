package control

import (
	"context"
	"fmt"
	"strings"

	"reasonix/internal/agent"
	"reasonix/internal/config"
	"reasonix/internal/i18n"
)

// resolveTurnImages resolves each user attachment once. Text-only parents keep
// the data only as candidates for a vision-capable child; vision-capable
// parents reuse the same data URLs for their own provider request.
func (c *Controller) resolveTurnImages(line string) (userImages, imageCandidates []string) {
	imageCandidates = c.resolveInputImageCandidates(line)
	if c.imageInputEnabled() {
		userImages = imageCandidates
	}
	return userImages, imageCandidates
}

func (c *Controller) prepareOrchestratedTurnImages(turn orchestratedTurn) orchestratedTurn {
	turn.userImages, turn.imageCandidates = c.resolveTurnImages(turn.imageReferenceInput())
	turn.imagesResolved = true
	return turn
}

func (c *Controller) imagesForOrchestratedTurn(ctx context.Context, turn orchestratedTurn) (userImages, imageCandidates []string) {
	if turn.imagesResolved {
		return turn.userImages, turn.imageCandidates
	}
	if turn.goalContinuation != nil {
		// A Goal continuation belongs to the same visible user turn, so keep its
		// child-only image candidates. Do not add them to the synthetic parent
		// message: a vision parent already has the image in its earlier history.
		return nil, agent.SubagentImageCandidates(ctx)
	}
	return c.resolveTurnImages(turn.imageReferenceInput())
}

func (c *Controller) withTurnImages(ctx context.Context, line string) context.Context {
	userImages, imageCandidates := c.resolveTurnImages(line)
	ctx = agent.WithUserImages(ctx, userImages)
	return c.withVisionRouting(agent.WithSubagentImageCandidates(ctx, imageCandidates))
}

// withVisionRouting names the model that reads an attachment this turn's own
// model cannot. Without it the candidates reach whichever sub-agent runs next,
// whose model was chosen for cost or speed and usually drops them on the wire.
func (c *Controller) withVisionRouting(ctx context.Context) context.Context {
	cfg, err := config.LoadForRoot(c.workspaceRoot)
	if err != nil || cfg == nil || strings.TrimSpace(cfg.Agent.VisionModel) == "" {
		return ctx
	}
	return agent.WithVisionRouting(ctx, cfg.Agent.VisionModel, func(child string) bool {
		if strings.TrimSpace(child) == "" {
			child = c.modelRef
		}
		entry, ok := cfg.ResolveModel(child)
		return ok && config.EffectiveVision(entry)
	})
}

func (turn orchestratedTurn) imageReferenceInput() string {
	if strings.TrimSpace(turn.imageRefs) != "" {
		return turn.imageRefs
	}
	return turn.raw
}

func (c *Controller) runTurnLoopWithImageRefsRawDisplay(ctx context.Context, input, raw, imageRefs, display string) error {
	return newTurnOrchestrator(c).runTurnLoopWithImageRefsRawDisplay(ctx, input, raw, imageRefs, display)
}

func (c *Controller) runTurnLoopWithFrozenImagesRawDisplay(ctx context.Context, input, raw, display string, images []string) error {
	return newTurnOrchestrator(c).runTurnLoopWithFrozenImagesRawDisplay(ctx, input, raw, display, images)
}

func (c *Controller) runEditedGoalLoopWithImageRefsRawDisplay(ctx context.Context, input, raw, imageRefs, display, original string) error {
	return newTurnOrchestrator(c).runEditedGoalLoopWithImageRefsRawDisplay(ctx, input, raw, imageRefs, display, original)
}

// A turn can carry images the current model cannot read. The data is already
// resolved and handed down as subagent candidates, so a delegated read is the
// one way to look at it — but the model has to be told the images are there, or
// it answers as though nothing was attached. This rides the turn tail, never the
// cache-stable prefix, the same way memory and job notes do.
const imageRoutingTag = "attached-images"

func imageRoutingNote(n int) string {
	if n <= 0 {
		return ""
	}
	return fmt.Sprintf(
		"<%s>\nThe user attached %d image(s). This model cannot read images, so they are not in your context.\n"+
			"Delegate with read_only_task to inspect them: the attachments are handed to the sub-agent automatically.\n"+
			"Never answer as if no image was attached, and never guess what it shows.\n</%s>\n\n",
		imageRoutingTag, n, imageRoutingTag,
	)
}

// bindOrchestratedTurnImages resolves the turn's attachments, binds them both
// for the model and for any delegate, and reports how many the model itself
// cannot read.
func (c *Controller) bindOrchestratedTurnImages(ctx context.Context, turn orchestratedTurn) (context.Context, []string, int) {
	userImages, candidates := c.imagesForOrchestratedTurn(ctx, turn)
	ctx = agent.WithUserImages(ctx, userImages)
	ctx = agent.WithSubagentImageCandidates(ctx, candidates)
	return ctx, userImages, unreadableImages(userImages, candidates)
}

// unreadableImages counts what the user attached and this model cannot see.
func unreadableImages(userImages, candidates []string) int {
	if len(userImages) > 0 {
		return 0
	}
	return len(candidates)
}

// imageRoutingPrefix also tells the user where their attachment went. Silence is
// the failure that matters: a pasted screenshot that reaches nothing reads as
// the product ignoring it.
func (c *Controller) imageRoutingPrefix(unreadable int) string {
	if unreadable <= 0 {
		return ""
	}
	c.notice(fmt.Sprintf(i18n.M.ImagesNotReadable, unreadable))
	return imageRoutingNote(unreadable)
}

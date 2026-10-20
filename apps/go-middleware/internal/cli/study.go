package cli

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/composition"
	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/platform/config"
)

// The study commands are the ones that were HTTP-only until now: what is due, and
// recording that you reviewed it. They are thin on purpose — every rule lives in
// the use cases the server already uses.

// runDue lists the cards whose next review is now or earlier.
func runDue(ctx context.Context, args []string, opts options, stdout, stderr io.Writer) error {
	fs := newFlagSet("due", "List the flashcards whose next review is now or earlier.", stderr)
	if err := parseFlags(fs, args, stdout); err != nil {
		return err
	}
	if err := rejectExtraArgs(fs); err != nil {
		return err
	}

	cfg, err := loadConfigAndAnnounce(opts, stderr)
	if err != nil {
		return err
	}

	study, closeDB, err := openStudy(ctx, cfg, true)
	if err != nil {
		return err
	}
	defer closeDB()

	due, err := study.GetDueCards.Execute(ctx)
	if err != nil {
		return err
	}

	if len(due) > 0 {
		writer := tabwriter.NewWriter(stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(writer, "ID\tTOPIC\tCONCEPT\tFRONT")

		for _, card := range due {
			fmt.Fprintf(writer, "%s\t%s\t%s\t%s\n",
				card.FlashcardID, card.TopicName, card.ConceptTitle, card.Front)
		}
		if err := writer.Flush(); err != nil {
			return err
		}
		fmt.Fprintln(stdout)
	}

	fmt.Fprintf(stdout, "%d cards due\n", len(due))
	return nil
}

// runReview records one review and prints the schedule it produced.
func runReview(ctx context.Context, args []string, opts options, stdout, stderr io.Writer) error {
	fs := newFlagSet("review",
		"Record a review of one flashcard and print the new schedule.", stderr)
	grade := fs.Int("grade", 0, "1 again, 2 hard, 3 good, 4 easy; required")
	durationMS := fs.Int("duration-ms", 0, "how long the review took, in milliseconds")
	if err := parseFlags(fs, args, stdout); err != nil {
		return err
	}

	flashcardID := strings.TrimSpace(strings.Join(fs.Args(), " "))
	if flashcardID == "" || *grade == 0 {
		fs.Usage()
		return errUsage
	}

	cfg, err := loadConfigAndAnnounce(opts, stderr)
	if err != nil {
		return err
	}

	// Recording a review is the one study command that writes, so it opens the
	// working database and applies pending migrations.
	study, closeDB, err := openStudy(ctx, cfg, false)
	if err != nil {
		return err
	}
	defer closeDB()

	state, err := study.SubmitReview.Execute(ctx, flashcardID, *grade, *durationMS)
	if err != nil {
		return err
	}

	fmt.Fprintf(stdout, "reviewed %s: next review %s (stability %.2f, difficulty %.2f)\n",
		state.FlashcardID, state.NextReview.Format(time.RFC3339),
		state.Stability, state.Difficulty)
	return nil
}

// runTopics lists the topics and how many concepts each one holds.
func runTopics(ctx context.Context, args []string, opts options, stdout, stderr io.Writer) error {
	fs := newFlagSet("topics", "List the topics and how many concepts each one holds.", stderr)
	if err := parseFlags(fs, args, stdout); err != nil {
		return err
	}
	if err := rejectExtraArgs(fs); err != nil {
		return err
	}

	cfg, err := loadConfigAndAnnounce(opts, stderr)
	if err != nil {
		return err
	}

	study, closeDB, err := openStudy(ctx, cfg, true)
	if err != nil {
		return err
	}
	defer closeDB()

	topics, err := study.ListConcepts.Execute(ctx)
	if err != nil {
		return err
	}

	if len(topics) > 0 {
		writer := tabwriter.NewWriter(stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(writer, "ID\tNAME\tCONCEPTS")

		for _, topic := range topics {
			fmt.Fprintf(writer, "%s\t%s\t%d\n", topic.ID, topic.Name, len(topic.Concepts))
		}
		if err := writer.Flush(); err != nil {
			return err
		}
		fmt.Fprintln(stdout)
	}

	fmt.Fprintf(stdout, "%d topics\n", len(topics))
	return nil
}

// runConcepts lists the concepts of one topic, or of every topic when no name is
// given. A name that matches nothing is an error rather than an empty list: a typo
// must not look like an empty topic.
func runConcepts(ctx context.Context, args []string, opts options, stdout, stderr io.Writer) error {
	fs := newFlagSet("concepts", "List the concepts of one topic, or of every topic.", stderr)
	if err := parseFlags(fs, args, stdout); err != nil {
		return err
	}

	topic := strings.TrimSpace(strings.Join(fs.Args(), " "))

	cfg, err := loadConfigAndAnnounce(opts, stderr)
	if err != nil {
		return err
	}

	study, closeDB, err := openStudy(ctx, cfg, true)
	if err != nil {
		return err
	}
	defer closeDB()

	topics, err := study.ListConcepts.Execute(ctx)
	if err != nil {
		return err
	}

	total := 0
	matched := topic == ""
	type conceptRow struct{ id, topic, title, file string }
	rows := make([]conceptRow, 0, len(topics))

	for _, tree := range topics {
		if topic != "" && !matchesTopic(tree.Name, tree.ID, topic) {
			continue
		}
		matched = true

		for _, concept := range tree.Concepts {
			total++
			rows = append(rows, conceptRow{concept.ID, tree.Name, concept.Title, concept.FilePath})
		}
	}

	// Checked before anything is printed, so a typo does not leave a table header
	// on stdout above the error. A topic that exists with no concepts is not this
	// case: it matched, and it answers zero.
	if !matched {
		return fmt.Errorf("no topic named %q", topic)
	}

	if len(rows) > 0 {
		writer := tabwriter.NewWriter(stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(writer, "ID\tTOPIC\tTITLE\tFILE")

		for _, row := range rows {
			fmt.Fprintf(writer, "%s\t%s\t%s\t%s\n", row.id, row.topic, row.title, row.file)
		}
		if err := writer.Flush(); err != nil {
			return err
		}
		fmt.Fprintln(stdout)
	}

	fmt.Fprintf(stdout, "%d concepts\n", total)
	return nil
}

// matchesTopic reports whether want names topic, by name or by identifier.
func matchesTopic(name, id, want string) bool {
	return strings.EqualFold(name, want) || id == want
}

// openStudy opens the session's database and wires the study context over it.
//
// The read-only commands must not create a database: answering "0 cards due" for a
// file that is not there is the same lie `search` refuses to tell, and it would
// also write a file as a side effect of a read.
func openStudy(
	ctx context.Context,
	cfg *config.Config,
	readOnly bool,
) (*composition.Study, func(), error) {
	var (
		sqlDB *sql.DB
		err   error
	)

	if readOnly {
		sqlDB, err = composition.OpenReadOnlyDatabase(ctx, cfg)
	} else {
		sqlDB, err = composition.OpenDatabase(ctx, cfg)
	}
	if err != nil {
		return nil, nil, err
	}

	return composition.NewStudy(sqlDB), func() { sqlDB.Close() }, nil
}

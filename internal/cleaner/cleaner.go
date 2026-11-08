package cleaner

import "context"

type Item struct {
	ID, Title string
	Protected bool
}
type Result struct {
	Item Item
	Err  error
}

func Preview(items []Item, selected map[string]bool, all bool) []Item {
	result := make([]Item, 0)
	for _, item := range items {
		if item.ID != "" && !item.Protected && (all || selected[item.ID]) {
			result = append(result, item)
		}
	}
	return result
}
func Run(ctx context.Context, items []Item, apply func(context.Context, string) error, progress func(Result)) []Result {
	results := make([]Result, 0, len(items))
	for _, item := range items {
		if ctx.Err() != nil {
			break
		}
		if item.ID == "" || item.Protected {
			continue
		}
		r := Result{Item: item, Err: apply(ctx, item.ID)}
		results = append(results, r)
		if progress != nil {
			progress(r)
		}
		if r.Err != nil {
			break
		}
	}
	return results
}

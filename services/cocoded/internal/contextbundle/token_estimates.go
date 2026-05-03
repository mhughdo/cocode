package contextbundle

func EstimateContentTokens(content string) int64 {
	if content == "" {
		return 0
	}
	return int64((len(content) + 3) / 4)
}

func EstimateItemTokens(item Item) int64 {
	if item.Content != "" {
		return EstimateContentTokens(item.Content)
	}
	if item.TokenEstimate > 0 {
		return item.TokenEstimate
	}
	return EstimateContentTokens(item.Title)
}

func EstimateBundleTokens(items []Item) int64 {
	var total int64
	for _, item := range items {
		total += EstimateItemTokens(item)
	}
	return total
}

func ApplyItemTokenEstimate(item Item) Item {
	item.TokenEstimate = EstimateItemTokens(item)
	return item
}

func ApplyBundleTokenEstimates(bundle Bundle) Bundle {
	for index := range bundle.Items {
		bundle.Items[index] = ApplyItemTokenEstimate(bundle.Items[index])
	}
	bundle.ItemCount = int64(len(bundle.Items))
	bundle.TokenEstimate = EstimateBundleTokens(bundle.Items)
	return bundle
}

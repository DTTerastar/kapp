package diff

import (
	"fmt"

	ctlres "github.com/k14s/kapp/pkg/kapp/resources"
)

const (
	resourceWithHistoryDebug = false

	appliedResAnnKey         = "kapp.k14s.io/original"
	appliedResDiffAnnKey     = "kapp.k14s.io/original-diff" // useful for debugging
	appliedResDiffMD5AnnKey  = "kapp.k14s.io/original-diff-md5"
	appliedResDiffFullAnnKey = "kapp.k14s.io/original-diff-full" // useful for debugging
)

type ResourceWithHistory struct {
	resource      ctlres.Resource
	changeFactory *ChangeFactory
}

func NewResourceWithHistory(resource ctlres.Resource, changeFactory *ChangeFactory) ResourceWithHistory {
	return ResourceWithHistory{resource.DeepCopy(), changeFactory}
}

func (r ResourceWithHistory) HistorylessResource() (ctlres.Resource, error) {
	return r.resourceWithoutHistoryAnnotations(r.resource)
}

func (r ResourceWithHistory) LastAppliedResource() ctlres.Resource {
	prevChange, prevDiffMD5, prevDiff := r.lastAppliedInfo()
	if prevChange != nil {
		md5Matches := prevChange.OpsDiff().MinimalMD5() == prevDiffMD5

		if resourceWithHistoryDebug {
			fmt.Printf("%s: md5 matches (%t) new %s prev %s\n ----> last applied\n%s\n----> new\n%s\n",
				r.resource.Description(),
				md5Matches, prevChange.OpsDiff().MinimalMD5(),
				prevDiffMD5, prevDiff, prevChange.OpsDiff().MinimalString())
		}

		if md5Matches {
			return prevChange.AppliedResource()
		}
	}
	return nil
}

func (r ResourceWithHistory) RecordLastAppliedResource(appliedRes ctlres.Resource) (ctlres.Resource, error) {
	change, err := r.newExactHistorylessChange(r.resource, appliedRes)
	if err != nil {
		return nil, err
	}

	appliedResBytes, err := change.AppliedResource().AsYAMLBytes()
	if err != nil {
		return nil, err
	}

	diff := change.OpsDiff()

	if resourceWithHistoryDebug {
		fmt.Printf("%s: recording md5 %s\n---> \n%s\n",
			r.resource.Description(), diff.MinimalMD5(), diff.MinimalString())
	}

	t := ctlres.StringMapAppendMod{
		ResourceMatcher: ctlres.AllResourceMatcher{},
		Path:            ctlres.NewPathFromStrings([]string{"metadata", "annotations"}),
		KVs: map[string]string{
			appliedResAnnKey:         string(appliedResBytes),
			appliedResDiffAnnKey:     diff.MinimalString(),
			appliedResDiffMD5AnnKey:  diff.MinimalMD5(),
			appliedResDiffFullAnnKey: diff.FullString(),
		},
	}

	resultRes := r.resource.DeepCopy()

	err = t.Apply(resultRes)
	if err != nil {
		return nil, err
	}

	return resultRes, nil
}

func (r ResourceWithHistory) lastAppliedInfo() (Change, string, string) {
	lastAppliedResBytes := r.resource.Annotations()[appliedResAnnKey]
	lastAppliedDiff := r.resource.Annotations()[appliedResDiffAnnKey]
	lastAppliedDiffMD5 := r.resource.Annotations()[appliedResDiffMD5AnnKey]

	if len(lastAppliedResBytes) == 0 || len(lastAppliedDiffMD5) == 0 {
		return nil, "", ""
	}

	prevNewRes, err := ctlres.NewResourceFromBytes([]byte(lastAppliedResBytes))
	if err != nil {
		return nil, "", ""
	}

	prevChange, err := r.newExactHistorylessChange(r.resource, prevNewRes)
	if err != nil {
		return nil, "", ""
	}

	return prevChange, lastAppliedDiffMD5, lastAppliedDiff
}

func (r ResourceWithHistory) newExactHistorylessChange(existingRes, newRes ctlres.Resource) (Change, error) {
	// If annotations are not removed line numbers will be mismatched
	existingRes, err := r.resourceWithoutHistoryAnnotations(existingRes)
	if err != nil {
		return nil, err
	}

	newRes, err = r.resourceWithoutHistoryAnnotations(newRes)
	if err != nil {
		return nil, err
	}

	return r.changeFactory.NewExactChange(existingRes, newRes)
}

func (ResourceWithHistory) resourceWithoutHistoryAnnotations(res ctlres.Resource) (ctlres.Resource, error) {
	res = res.DeepCopy()

	// TODO remove since switched to ops diffing
	// If annotations are not removed line numbers will be mismatched
	for _, t := range removeAppliedResAnnKeysMods() {
		err := t.Apply(res)
		if err != nil {
			return nil, err
		}
	}

	return res, nil
}

func removeAppliedResAnnKeysMods() []ctlres.ResourceMod {
	return []ctlres.ResourceMod{
		ctlres.FieldRemoveMod{
			ResourceMatcher: ctlres.AllResourceMatcher{},
			Path:            ctlres.NewPathFromStrings([]string{"metadata", "annotations", appliedResAnnKey}),
		},
		ctlres.FieldRemoveMod{
			ResourceMatcher: ctlres.AllResourceMatcher{},
			Path:            ctlres.NewPathFromStrings([]string{"metadata", "annotations", appliedResDiffAnnKey}),
		},
		ctlres.FieldRemoveMod{
			ResourceMatcher: ctlres.AllResourceMatcher{},
			Path:            ctlres.NewPathFromStrings([]string{"metadata", "annotations", appliedResDiffMD5AnnKey}),
		},
		ctlres.FieldRemoveMod{
			ResourceMatcher: ctlres.AllResourceMatcher{},
			Path:            ctlres.NewPathFromStrings([]string{"metadata", "annotations", appliedResDiffFullAnnKey}),
		},
	}
}

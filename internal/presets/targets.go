package presets

import "github.com/IRevolve/Bear/internal/config"

// Targets enthält alle vordefinierten Target-Konfigurationen
var Targets = map[string]config.TargetTemplate{
	"docker": {
		Name: "docker",
		Defaults: map[string]string{
			"REGISTRY": "docker.io",
		},
		Deploy: []config.Step{
			{Name: "Build image", Run: "docker build -t $REGISTRY/$NAME:$VERSION ."},
			{Name: "Push image", Run: "docker push $REGISTRY/$NAME:$VERSION"},
		},
	},
	"cloudrun": {
		Name: "cloudrun",
		Defaults: map[string]string{
			"REGION": "europe-west1",
			"MEMORY": "512Mi",
		},
		Deploy: []config.Step{
			{Name: "Build", Run: "docker build -t gcr.io/$PROJECT/$NAME:$VERSION ."},
			{Name: "Push", Run: "docker push gcr.io/$PROJECT/$NAME:$VERSION"},
			{Name: "Deploy", Run: "gcloud run deploy $NAME --image gcr.io/$PROJECT/$NAME:$VERSION --region $REGION --memory $MEMORY"},
		},
	},
	"cloudrun-job": {
		Name: "cloudrun-job",
		Defaults: map[string]string{
			"REGION": "europe-west1",
			"MEMORY": "512Mi",
		},
		Deploy: []config.Step{
			{Name: "Build", Run: "docker build -t gcr.io/$PROJECT/$NAME:$VERSION ."},
			{Name: "Push", Run: "docker push gcr.io/$PROJECT/$NAME:$VERSION"},
			{Name: "Deploy", Run: "gcloud run jobs replace job.yaml --region $REGION"},
		},
	},
	"lambda": {
		Name: "lambda",
		Defaults: map[string]string{
			"REGION":  "eu-central-1",
			"RUNTIME": "provided.al2",
			"MEMORY":  "128",
		},
		Deploy: []config.Step{
			{Name: "Package", Run: "zip -r function.zip ."},
			{Name: "Deploy", Run: "aws lambda update-function-code --function-name $NAME --zip-file fileb://function.zip --region $REGION"},
		},
	},
	"s3": {
		Name: "s3",
		Defaults: map[string]string{
			"REGION": "eu-central-1",
		},
		Deploy: []config.Step{
			{Name: "Sync", Run: "aws s3 sync ./dist s3://$BUCKET --delete"},
		},
	},
	"s3-static": {
		Name: "s3-static",
		Defaults: map[string]string{
			"REGION": "eu-central-1",
		},
		Deploy: []config.Step{
			{Name: "Build", Run: "npm run build"},
			{Name: "Sync", Run: "aws s3 sync ./dist s3://$BUCKET --delete"},
			{Name: "Invalidate", Run: "aws cloudfront create-invalidation --distribution-id $CF_DIST --paths '/*'"},
		},
	},
	"kubernetes": {
		Name: "kubernetes",
		Defaults: map[string]string{
			"NAMESPACE": "default",
		},
		Deploy: []config.Step{
			{Name: "Build", Run: "docker build -t $REGISTRY/$NAME:$VERSION ."},
			{Name: "Push", Run: "docker push $REGISTRY/$NAME:$VERSION"},
			{Name: "Apply", Run: "kubectl set image deployment/$NAME $NAME=$REGISTRY/$NAME:$VERSION -n $NAMESPACE"},
		},
	},
	"helm": {
		Name: "helm",
		Defaults: map[string]string{
			"NAMESPACE": "default",
		},
		Deploy: []config.Step{
			{Name: "Build", Run: "docker build -t $REGISTRY/$NAME:$VERSION ."},
			{Name: "Push", Run: "docker push $REGISTRY/$NAME:$VERSION"},
			{Name: "Upgrade", Run: "helm upgrade --install $NAME ./chart --set image.tag=$VERSION -n $NAMESPACE"},
		},
	},
	"fly": {
		Name: "fly",
		Deploy: []config.Step{
			{Name: "Deploy", Run: "fly deploy --image $NAME:$VERSION"},
		},
	},
	"vercel": {
		Name: "vercel",
		Deploy: []config.Step{
			{Name: "Deploy", Run: "vercel --prod"},
		},
	},
	"netlify": {
		Name: "netlify",
		Deploy: []config.Step{
			{Name: "Build", Run: "npm run build"},
			{Name: "Deploy", Run: "netlify deploy --prod --dir=dist"},
		},
	},
}

// GetTarget gibt ein vordefiniertes Target zurück
func GetTarget(name string) (config.TargetTemplate, bool) {
	target, ok := Targets[name]
	return target, ok
}

// ListTargets gibt alle verfügbaren Targets zurück
func ListTargets() []string {
	var names []string
	for name := range Targets {
		names = append(names, name)
	}
	return names
}

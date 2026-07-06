# AI Model Approvals

Date: 2026-06-07

This document records the dependency/model choices and approval status for Cartolensia local AI inference.

## 2026-06-07 Implementation Status

The later implementation run received explicit approval for the dependencies and model weights listed below. They were installed/downloaded only into repo-local ignored paths:

- Python environment: `.cartolensia/ai-venv`.
- Model cache: `.cartolensia/models`.
- OpenCV YuNet: `.cartolensia/models/opencv/face_detection_yunet_2023mar.onnx`.
- Hugging Face cache: `.cartolensia/models/huggingface`.
- OpenCLIP cache: `.cartolensia/models/openclip`.

Installed packages:

- `torch`, `torchvision` from the official CUDA 12.8 PyTorch wheel index.
- `opencv-python-headless`.
- `transformers`.
- `safetensors`.
- `open-clip-torch`.
- `facenet-pytorch` and transitive dependencies.

Downloaded/verified models:

- Torchvision EfficientNet-B0 and MobileNetV3 Large ImageNet weights.
- OpenCV YuNet ONNX face detector.
- Falconsai `nsfw_image_detection`.
- OpenCLIP `laion/CLIP-ViT-B-32-laion2B-s34B-b79K`.
- Salesforce `blip-image-captioning-base`.

Live real-peek AI validation was explicitly approved for the current 54 indexed assets only. It processed 48 photo assets, skipped 4 tracks and 2 videos, and stored predictions/metadata in PostgreSQL only. No AI model, cache, temporary input, prediction output, export, or generated file was placed under `/mnt/Models/rclone`.

Safety baseline:

- Model files and caches must live under `.cartolensia/models` or another configured non-archive path.
- No model file, temporary input, generated output, embedding cache, or export may be placed under `/mnt/Models/rclone`.
- AI jobs must run only on explicitly selected/bounded scopes.
- Model licenses and training-data provenance are evaluated separately. A permissive package license is not enough by itself.

## Recommended Staging

1. Install CUDA PyTorch/torchvision into the existing AI venv only after approval.
2. Implement real image classification first with torchvision MobileNetV3 or EfficientNet-B0.
3. Implement face detection with OpenCV YuNet first because the model is small and its OpenCV provenance is clearer than facenet-pytorch's ported face-recognition weights.
4. Add NSFW/safety classification only after explicit approval of the Falconsai model and its Hugging Face weight provenance.
5. Add OpenCLIP embeddings after accepting LAION-trained weight provenance and the larger download.
6. Defer BLIP captioning unless the user explicitly accepts the size and research-use limitations.

## Candidate Table

| Capability | Candidate | Package/model | Code license | Weight license/provenance | Approx download size | CUDA support | CPU fallback | Exact install/download command after approval | Risk notes | Recommendation |
| --- | --- | --- | --- | --- | ---: | --- | --- | --- | --- | --- |
| Base runtime | PyTorch CUDA stack | `torch`, `torchvision` from official PyTorch CUDA 12.8 wheel index | BSD-style PyTorch project license; verify local wheel metadata after install | No model weights in package install | Large, likely multiple GB with CUDA wheel dependencies | Yes, expected for RTX 3090 Ti native path | Yes | `.cartolensia/ai-venv/bin/python -m pip install torch torchvision --index-url https://download.pytorch.org/whl/cu128` | Large download; must not be installed in this preflight. Official PyTorch page says choose the CUDA platform suited to the machine. | Approve first if real GPU inference is desired. |
| Classification | Torchvision MobileNetV3 Large | `torchvision.models.mobilenet_v3_large(weights=MobileNet_V3_Large_Weights.DEFAULT)` | Torchvision BSD-3-Clause | ImageNet-1K weights distributed by torchvision; dataset terms/provenance should be documented | 21.1 MB weights | Yes through PyTorch | Yes | `.cartolensia/ai-venv/bin/python -c "from torchvision.models import mobilenet_v3_large, MobileNet_V3_Large_Weights; mobilenet_v3_large(weights=MobileNet_V3_Large_Weights.DEFAULT)"` | General ImageNet categories, not a photo-library taxonomy; good first smoke model. | Recommended first classifier. |
| Classification | Torchvision EfficientNet-B0 | `torchvision.models.efficientnet_b0(weights=EfficientNet_B0_Weights.DEFAULT)` | Torchvision BSD-3-Clause | ImageNet-1K weights ported/distributed by torchvision; dataset terms/provenance should be documented | 20.5 MB weights | Yes through PyTorch | Yes | `.cartolensia/ai-venv/bin/python -c "from torchvision.models import efficientnet_b0, EfficientNet_B0_Weights; efficientnet_b0(weights=EfficientNet_B0_Weights.DEFAULT)"` | Slightly stronger than MobileNetV3 but not materially more product-specific. | Alternative first classifier. |
| Face detection | OpenCV YuNet ONNX | `opencv-python-headless` + `face_detection_yunet_2023mar.onnx` from `opencv_zoo` | OpenCV 4.5+ is Apache-2.0; verify PyPI wheel metadata after install | OpenCV zoo YuNet model; OpenCV provenance is clearer than many face packages | Model appears sub-MB; package wheel is larger | CPU OpenCV DNN path first; CUDA not required | Yes | `.cartolensia/ai-venv/bin/python -m pip install opencv-python-headless` then `mkdir -p .cartolensia/models/opencv && curl -L -o .cartolensia/models/opencv/face_detection_yunet_2023mar.onnx https://github.com/opencv/opencv_zoo/raw/main/models/face_detection_yunet/face_detection_yunet_2023mar.onnx` | Face detection is sensitive. Store detections privately, avoid public/person-identification claims. | Recommended first face detector. |
| Face detection/categorization | facenet-pytorch MTCNN | `facenet-pytorch` | PyPI metadata reports MIT | Project says face-recognition weights were ported from David Sandberg's TensorFlow facenet repo and references VGGFace2/CASIA-Webface; MTCNN weight provenance is not separated clearly enough for automatic approval | Package 1.9 MB; optional recognition weights 107-111 MB | Yes through PyTorch | Yes | `.cartolensia/ai-venv/bin/python -m pip install facenet-pytorch` | Useful, but model provenance and biometric implications need explicit approval. Do not enable identification/grouping by default. | Defer unless user approves. |
| NSFW/safety | Falconsai NSFW image detection | `transformers` model `Falconsai/nsfw_image_detection` | Model card reports Apache-2.0 | Hugging Face model weights under same model license; trained data provenance should be accepted explicitly | Estimated 300-350 MB for ViT/safetensors assets | Yes through PyTorch/Transformers | Yes | `.cartolensia/ai-venv/bin/python -m pip install transformers safetensors` then `.cartolensia/ai-venv/bin/python -c "from transformers import pipeline; pipeline('image-classification', model='Falconsai/nsfw_image_detection', cache_dir='.cartolensia/models/huggingface')"` | NSFW labels can be wrong and sensitive; UI must label results as predictions and keep workflow opt-in. | Approve explicitly before use. |
| Embeddings/search | OpenCLIP ViT-B/32 LAION-2B | `open-clip-torch`, `laion/CLIP-ViT-B-32-laion2B-s34B-b79K` or equivalent HF mirror | `open-clip-torch` package metadata reports MIT | Hugging Face mirror reports MIT; model trained on LAION-2B English subset, which carries provenance/privacy risk | Around 0.2B params; expect roughly 600-700 MB model assets | Yes through PyTorch | Yes, slower | `.cartolensia/ai-venv/bin/python -m pip install open-clip-torch` then `.cartolensia/ai-venv/bin/python -c "import open_clip; open_clip.create_model_and_transforms('hf-hub:laion/CLIP-ViT-B-32-laion2B-s34B-b79K', cache_dir='.cartolensia/models/openclip')"` | Good for image/text search; training data provenance should be accepted before download. | Second-stage after classifier. |
| Captioning | BLIP base captioning | `transformers` model `Salesforce/blip-image-captioning-base` | Model card reports BSD-3-Clause | BLIP model card says base image-captioning model; includes research/ethical caveats | Roughly 900 MB to 1 GB | Yes through PyTorch/Transformers | Yes, slower | `.cartolensia/ai-venv/bin/python -m pip install transformers safetensors` then `.cartolensia/ai-venv/bin/python -c "from transformers import AutoProcessor, AutoModelForImageTextToText; AutoProcessor.from_pretrained('Salesforce/blip-image-captioning-base', cache_dir='.cartolensia/models/huggingface'); AutoModelForImageTextToText.from_pretrained('Salesforce/blip-image-captioning-base', cache_dir='.cartolensia/models/huggingface')"` | Heavier and likely slower; generated captions need review and should not be treated as truth. | Defer unless explicitly requested. |
| Music-to-MIDI | Basic Pitch | `basic-pitch` plus optional `pretty_midi`/`mido` summary readers | Basic Pitch project reports Apache-2.0; verify local package metadata after install | Basic Pitch model assets are packaged/loaded by the library; record local package/version metadata in offline manifests | Moderate Python package/model payload | TensorFlow/CoreML-oriented provider; CPU baseline available | Yes | `.cartolensia/ai-venv/bin/python -m pip install basic-pitch pretty_midi mido` | Practical polyphonic transcription baseline. Treat MIDI as inferred metadata, not ground truth. Upstream package compatibility currently targets Python 3.7-3.11; Python 3.12+ bundles should use a reviewed compatible archive or expose a missing-component state. | Approved for optional MIDI transcription jobs. |
| Music stems | Demucs | `demucs` | Demucs package/project reports MIT; verify local package metadata after install | Model checkpoints are provider-specific and should be recorded in the component manifest/cache | Large model/checkpoint plus potentially large generated stem cache | Yes through PyTorch when compatible GPU runtime is installed | Yes, slower | `.cartolensia/ai-venv/bin/python -m pip install demucs` | Stem outputs can be large; run on demand or on selected scopes and keep cache cleanup available. | Approved for selected/on-demand stem separation. |
| Advanced multi-instrument transcription | MT3-style provider | Future optional `music-mt3` component | Project/model terms must be reviewed before enabling | Model weights are larger and provider-specific | Large | Depends on provider | Usually yes, slow | Provide as an offline component/archive after review. | Better orchestral/electronic instrument transcription target, but too heavy for the default path. | Future optional provider, not required for release baseline. |
| Vector store fallback | Local JSON/PostgreSQL vector fallback | Go/PostgreSQL implementation | Project AGPL-3.0-or-later | No model weights | None | N/A | Yes | No install command. Implement `VectorStore` with JSON float vectors and brute-force cosine for small local sets; keep pgvector optional. | Slower, but safe and functional for small datasets. | Recommended before pgvector dependency. |

## Sources Checked

- PyTorch installation selector: https://pytorch.org/get-started/locally/
- Torchvision license: https://github.com/pytorch/vision/blob/main/LICENSE
- Torchvision MobileNetV3 weights/docs: https://docs.pytorch.org/vision/0.15/models/generated/torchvision.models.mobilenet_v3_large.html
- Torchvision EfficientNet-B0 weights/docs: https://docs.pytorch.org/vision/main/models/generated/torchvision.models.efficientnet_b0
- facenet-pytorch PyPI metadata: https://pypi.org/project/facenet-pytorch/
- OpenCV license: https://opencv.org/license/
- OpenCV YuNet model docs: https://github.com/opencv/opencv_zoo/blob/main/models/face_detection_yunet/README.md
- Falconsai NSFW model card: https://huggingface.co/Falconsai/nsfw_image_detection
- open-clip-torch PyPI metadata: https://pypi.org/project/open-clip-torch/
- CLIP ViT-B/32 LAION model card mirror checked during preflight: https://huggingface.co/rroset/CLIP-ViT-B-32-laion2B-s34B-b79K
- Salesforce BLIP captioning model card: https://huggingface.co/Salesforce/blip-image-captioning-base
- Basic Pitch project: https://github.com/spotify/basic-pitch
- Demucs project: https://github.com/facebookresearch/demucs
- MT3 project: https://github.com/magenta/mt3

## Approval Checklist

Approve or defer each item before the next long implementation run:

- Install CUDA PyTorch/torchvision into `.cartolensia/ai-venv`.
- Download MobileNetV3 or EfficientNet-B0 classification weights.
- Install OpenCV and download YuNet ONNX face detector.
- Approve or reject facenet-pytorch MTCNN/recognition provenance.
- Approve or reject Falconsai NSFW model license/provenance.
- Approve or reject OpenCLIP LAION embeddings.
- Approve or defer BLIP captioning.
- Approve whether AI may run on the current 54 real-peek assets after implementation, or restrict AI tests to synthetic fixtures only.

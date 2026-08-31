import json
import time

from alibabacloud_credentials.client import Client as CredentialClient
from alibabacloud_tea_openapi import models as open_api_models
from alibabacloud_tea_util import models as util_models
from alibabacloud_yike20260707.client import Client
from alibabacloud_yike20260707 import models


JOB_ID = "ag_86a3d0e5ef984f96958ea579448211c1"


def create_client():
    credential = CredentialClient()

    config = open_api_models.Config(
        credential=credential
    )
    config.region_id = "ap-southeast-1"
    config.endpoint = "yike.ap-southeast-1.aliyuncs.com"

    return Client(config)


def main():
    client = create_client()
    runtime = util_models.RuntimeOptions()

    while True:
        request = models.GetVideoGenerationJobRequest(
            job_id=JOB_ID
        )

        response = client.get_video_generation_job_with_options(
            request,
            runtime
        )

        job = response.body.video_generation_job
        print(f"任务状态：{job.status}")

        if job.status == "Finished":
            output = json.loads(job.output)
            print("生成结果：")
            print(json.dumps(output, ensure_ascii=False, indent=2))

            medias = output.get("Medias", [])
            for index, media in enumerate(medias, start=1):
                print(f"视频 {index}：{media.get('OutputUrl')}")
            break

        if job.status == "Failed":
            raise RuntimeError(
                f"视频生成失败：{job.error_message}"
            )

        if job.status not in {
            "Created",
            "Queuing",
            "Executing"
        }:
            raise RuntimeError(
                f"未知任务状态：{job.status}"
            )

        time.sleep(5)


if __name__ == "__main__":
    main()
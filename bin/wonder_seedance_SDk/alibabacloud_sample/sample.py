# -*- coding: utf-8 -*-
# This file is auto-generated, don't edit it. Thanks.
import os
import sys
import json

from typing import List

from alibabacloud_yike20260707.client import Client as Yike20260707Client
from alibabacloud_credentials.client import Client as CredentialClient
from alibabacloud_tea_openapi import models as open_api_models
from alibabacloud_yike20260707 import models as yike_20260707_models
from alibabacloud_tea_util import models as util_models
from alibabacloud_tea_util.client import Client as UtilClient


class Sample:
    def __init__(self):
        pass

    @staticmethod
    def create_client() -> Yike20260707Client:
        """
        Initialize the Client with the credentials
        @return: Client
        @throws Exception
        """
        # It is recommended to use the default credential. For more credentials, please refer to: https://help.aliyun.com/document_detail/378659.html.
        credential = CredentialClient()
        config = open_api_models.Config(
            credential=credential
        )
        # See https://api.alibabacloud.com/product/Yike.
        config.region_id = 'ap-southeast-1'
        config.endpoint = f'yike.ap-southeast-1.aliyuncs.com'
        # config.endpoint = f'yike.cn-shanghai.aliyuncs.com'
        return Yike20260707Client(config)

    @staticmethod
    def main(
        args: List[str],
    ) -> None:
        client = Sample.create_client()
        submit_video_generation_job_request = yike_20260707_models.SubmitVideoGenerationJobRequest(
            job_type='text_to_video',

            ## Wonder-Pro = 阿里部署的Seedance 2.0 Mini
            # model='Wonder-Pro',
            ## Wonder-Ultra = 阿里部署的Seedance 2.5
            model='Wonder-Ultra',
            ##Wonder-Standard = 阿里部署的Seedance 2.0 
            # model='Wonder-Standard,
            input='''{
  "Prompt": "一只橘猫在雨后的石板路上缓慢行走，电影感"
}'''
        )
        runtime = util_models.RuntimeOptions()
        try:
            resp = client.submit_video_generation_job_with_options(submit_video_generation_job_request, runtime)
            print(json.dumps(resp, default=str, indent=2))
        except Exception as error:
            # Only a printing example. Please be careful about exception handling and do not ignore exceptions directly in engineering projects.
            # print error message
            print(error.message)
            # Please click on the link below for diagnosis.
            print(error.data.get("Recommend"))

    @staticmethod
    async def main_async(
        args: List[str],
    ) -> None:
        client = Sample.create_client()
        submit_video_generation_job_request = yike_20260707_models.SubmitVideoGenerationJobRequest(
            job_type='text_to_video',
            model='Wonder-Pro',
            input='''{
  "Prompt": "一只橘猫在雨后的石板路上缓慢行走，电影感"
}'''
        )
        runtime = util_models.RuntimeOptions()
        try:
            resp = await client.submit_video_generation_job_with_options_async(submit_video_generation_job_request, runtime)
            print(json.dumps(resp, default=str, indent=2))
        except Exception as error:
            # Only a printing example. Please be careful about exception handling and do not ignore exceptions directly in engineering projects.
            # print error message
            print(error.message)
            # Please click on the link below for diagnosis.
            print(error.data.get("Recommend"))


if __name__ == '__main__':
    Sample.main(sys.argv[1:])

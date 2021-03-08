import argparse
import boto3
from os import listdir
from os import path
from os import fsync
import json
import copy
import subprocess

PRODUCTION_BUCKET_NAME = 'stacktape-infrastructure-modules'


def rewrite_file_content(file_handler, new_content):
    file_handler.seek(0)
    file_handler.write(new_content)
    file_handler.truncate()
    file_handler.flush()
    fsync(file_handler.fileno())


def modify_schema_content(original_content, subversion):
    modified_schema = json.loads(original_content)
    modified_schema['description'] = subversion
    return json.dumps(modified_schema)


def build_resource_package(relative_path_to_resource_directory, subversion):
    json_files = [f for f in listdir(
        relative_path_to_resource_directory) if f.endswith('.json')]
    # find json file (its name). only one should be present
    if len(json_files) != 1:
        raise Exception('There should be exactly one json file (schema) in {} directory'.format(
            relative_path_to_resource_directory))
    # load json file (schema)
    with open(path.join(relative_path_to_resource_directory, json_files[0]), 'r+') as schema_file:
        original_schema_content = schema_file.read()
        print('building package for {}'.format(
            relative_path_to_resource_directory))
        try:
            modified_schema_content = modify_schema_content(
                original_schema_content, subversion)
            rewrite_file_content(schema_file, modified_schema_content)
            subprocess.run(
                ['make'], cwd=relative_path_to_resource_directory).check_returncode()
            subprocess.run(['cfn', 'submit', '--dry-run'],
                           cwd=relative_path_to_resource_directory).check_returncode()

        except:
            print('building package for {} FAILED'.format(
                relative_path_to_resource_directory))
            rewrite_file_content(schema_file, original_schema_content)
            # after we rewrite content back, we need to regenerate cfn schema due to docs
            subprocess.run(['cfn', 'generate'],
                           cwd=relative_path_to_resource_directory).check_returncode()
            raise

        rewrite_file_content(schema_file, original_schema_content)
        # after we rewrite content back, we need to regenerate cfn schema due to docs
        subprocess.run(['cfn', 'generate'],
                       cwd=relative_path_to_resource_directory).check_returncode()
    print('building package for {} SUCCESS'.format(
        relative_path_to_resource_directory))


def check_subversion_existence(s3_client, bucket_name, major_version, subversion):
    if len(subversion) != 7 or not subversion.isnumeric():
        raise Exception('Invalid format of subversion {}'.format(subversion))
    full_version_prefix = 'atlasMongo/{}/{}'.format(major_version, subversion)
    response = s3_client.list_objects_v2(
        Bucket=bucket_name, Prefix=full_version_prefix)
    print(json.dumps(response, indent=2, default=str))
    if (not 'Contents' in response) or (len(response['Contents']) > 0):
        print('Prefix {} already exists in bucket {}'.format(
            full_version_prefix, bucket_name))
        return True
    return False


def upload_resource_package(relative_path_to_resource_directory, s3_client, bucket_name, major_version, subversion):
    full_version_prefix = 'atlasMongo/{}/{}'.format(major_version, subversion)
    zip_files = [f for f in listdir(
        relative_path_to_resource_directory) if f.endswith('.zip')]
    # find json file (its name). only one should be present
    if len(zip_files) != 1:
        raise Exception('There should be exactly one zip file (resource package) in {} directory'.format(
            relative_path_to_resource_directory))
    print('uploading {} into {}/{}/{}'.format(
        zip_files[0], bucket_name, full_version_prefix, zip_files[0]))
    s3_client.upload_file(Filename='{}/{}'.format(relative_path_to_resource_directory,
                                                  zip_files[0]), Bucket=bucket_name, Key='{}/{}'.format(full_version_prefix, zip_files[0]))
    print('upload success')


def resolve_resource_packages_for_major_version(s3_client, bucket_name, major_version, subversion):
    # for every directory in the cfn-version/<<major_version>> directory
    # each directory represents a resource
    for resource_dir_name in listdir(path.join('cfn-resources', major_version)):
        full_resource_dir = path.join(
            'cfn-resources', major_version, resource_dir_name)
        build_resource_package(
            relative_path_to_resource_directory=full_resource_dir, subversion=subversion)
        upload_resource_package(relative_path_to_resource_directory=full_resource_dir,
                                s3_client=s3_client, bucket_name=bucket_name, major_version=major_version, subversion=subversion)


parser = argparse.ArgumentParser()
parser.add_argument('--major-version', required=True)
parser.add_argument('--subversion', required=True)
parser.add_argument('--bucket-name', required=True)
parser.add_argument('--bucket-region', required=True)

args = vars(parser.parse_args())

s3 = boto3.client('s3', region_name=args['bucket_region'])

subversion_exists = check_subversion_existence(
    s3_client=s3, bucket_name=args['bucket_name'], major_version=args['major_version'], subversion=args['subversion'])

if subversion_exists and args['bucket_name'] == PRODUCTION_BUCKET_NAME:
    raise Exception('atlasMongo/{}/{} already exists in production bucket. You CANNOT override version in production bucket'.format(
        args['major_version'], args['subversion']))

resolve_resource_packages_for_major_version(
    s3_client=s3, bucket_name=args['bucket_name'], major_version=args['major_version'], subversion=args['subversion'])

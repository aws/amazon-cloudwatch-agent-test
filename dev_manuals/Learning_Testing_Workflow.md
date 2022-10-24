## Understanding Agent Testing Github Workflow
aws/amazon-cloudwatch-agent uses Github workflows to do various things. One of the things is running integration tests against each new agent code commit to the repository. Personal forks of agent code can do the same thing.

This section is written to help new contributors onboard before they have to dive deep into the code. Note that this document may become outdated, so only use this for initial education purposes.

### Descriptions of Shared Steps
- **MakeBinary**:
  - Make binaries for all supported OSs, cache them to not repeat this expensive step (~15 minutes) for when github_sha hasn’t changed, sign them all. Then The packages get uploaded to s3 for dependent steps to download and install agent. Docker image of the agent also gets built and added to the aws account’s ECR for dependent steps to use to run agent. Code for this is entirely in the agent repo.
- **GenerateTestMatrix**
  - Github workflow step defined in the agent repo checks out the test repository to use integration test package’s test_case_generator. The test generator creates a map of which test workflow step needs to run which set of test suites where each suite is defined at the go package level. Each suite is therefore expressed as a directory name that contains various tests in the suite. The output of this is stored in the workflow so that later steps can figure out which tests to run.
- **SignMacAndWindowsPackage**
  - These don’t have to happen for Mac and Windows at the same time, but this step combined the two. This takes MSI and MacPkg created by previous steps from s3, sign, and re-upload. 
  - #TODO Why is this part of integration test flow? Seems like this doesn’t serve any purpose for integration test.


### Linux

#### ECS
1. **MakeBinary, GenerateTestMatrix**
2. **ECSFargateIntegrationTest**

#### EC2

1. **MakeBinary, GenerateTestMatrix,** and **StartLocalStack.**
  1. Unlike ECS, **StartLocalStack** is needed because #TODO
2. **EC2LinuxIntegrationTest**
3. **StopLocalStack** because #TODO

*NOTE: no signing package for lInux? #TODO Why?

### Mac Github Workflow

1. **MakeBinary**
2. **MakeMacPkg.** Using binary created by **MakeBinary,** build mac packages and upload to s3. Code for this is entirely in the agent repo.
3. **SignMacAndWindowsPackage.**

### MSI Github Workflow

1. **MakeBinary**.
2. **MakeMSIZip.** Using windows zip created by**MakeBinary,** make some required changes for the next **BuildMSI** step, re-zip, and upload to s3. Code for this is entirely in the agent repo.
3. **BuildMSI.** Using the zip created and uploaded by **MakeMSIZip,** build an agent installer for windows (MSI). Code for this is entirely in the agent repo.
4. **SignMacAndWindowsPackage.**

### Nvidia GPU

1. **MakeBinary, GenerateTestMatrix** and **StartLocalStack**
2. **BuildMSI**. We run both Linux and Windows Tests
3. **EC2NvidiaGPUIntegrationTest**
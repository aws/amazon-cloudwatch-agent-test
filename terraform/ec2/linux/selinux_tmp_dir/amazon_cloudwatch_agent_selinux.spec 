# vim: sw=4:ts=4:et

%define relabel_files() \
    /sbin/restorecon -R /opt/aws/amazon-cloudwatch-agent; \
    /sbin/restorecon -R /usr/lib/systemd/system/amazon-cloudwatch-agent.service; \

%define selinux_policyver 3.13.1-266

Name:   amazon_cloudwatch_agent_selinux
Version:	1.0
Release:	1%{?dist}
Summary:	SELinux policy module for amazon_cloudwatch_agent

Group:	System Environment/Base
License:	GPLv2+
URL:		http://your-website.com/selinux/amazon_cloudwatch_agent_selinux
Source0:	amazon_cloudwatch_agent.pp
Source1:	amazon_cloudwatch_agent.if
Source2:	amazon_cloudwatch_agent_selinux.8


Requires: policycoreutils, libselinux-utils
Requires(post): selinux-policy-base >= %{selinux_policyver}, policycoreutils
Requires(postun): policycoreutils
BuildArch: noarch

%description
This package installs and sets up the SELinux policy security module for amazon_cloudwatch_agent.

%install
install -d %{buildroot}%{_datadir}/selinux/packages
install -m 644 %{SOURCE0} %{buildroot}%{_datadir}/selinux/packages
install -d %{buildroot}%{_datadir}/selinux/devel/include/contrib
install -m 644 %{SOURCE1} %{buildroot}%{_datadir}/selinux/devel/include/contrib/
install -d %{buildroot}%{_mandir}/man8/
install -m 644 %{SOURCE2} %{buildroot}%{_mandir}/man8/amazon_cloudwatch_agent_selinux.8
install -d %{buildroot}/etc/selinux/targeted/contexts/users/


%post
semodule -n -i %{_datadir}/selinux/packages/amazon_cloudwatch_agent.pp
if /usr/sbin/selinuxenabled ; then
    /usr/sbin/load_policy
    %relabel_files

fi;
exit 0

%postun
if [ $1 -eq 0 ]; then
    semodule -n -r amazon_cloudwatch_agent
    if /usr/sbin/selinuxenabled ; then
       /usr/sbin/load_policy
       %relabel_files

    fi;
fi;
exit 0

%files
%attr(0600,root,root) %{_datadir}/selinux/packages/amazon_cloudwatch_agent.pp
%{_datadir}/selinux/devel/include/contrib/amazon_cloudwatch_agent.if
%{_mandir}/man8/amazon_cloudwatch_agent_selinux.8.*

%changelog
* Mon Jan 2 2025 Parampreet Singh sipmrp@amazon.com
- Initial version
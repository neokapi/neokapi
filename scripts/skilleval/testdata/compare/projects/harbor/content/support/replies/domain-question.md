Hi Priya,

Your certificate is waiting for the CNAME record to resolve.

Run `harbor domains list <app>` to see the record Harbor expects. Once your DNS
provider serves it, Harbor requests the certificate within a few minutes.
DNS changes can take up to an hour to reach everyone.

If the domain still shows as pending after that, reply with the output of
`harbor domains list <app>` and I'll look at it.

Thanks,
Harbor support
